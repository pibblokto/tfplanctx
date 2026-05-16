package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/piblokto/tfplanctx/internal/codec"
	"github.com/piblokto/tfplanctx/internal/plan"
)

var noisyComputedTopLevelFields = map[string]struct{}{
	"id":               {},
	"etag":             {},
	"self_link":        {},
	"arn":              {},
	"fingerprint":      {},
	"generation":       {},
	"uid":              {},
	"resource_version": {},
	"unique_id":        {},
}

var defaultTopLevelFields = map[string]struct{}{
	"labels":      {},
	"annotations": {},
	"tags":        {},
	"timeouts":    {},
	"condition":   {},
	"metadata":    {},
}

type compactResourceRecord struct {
	Resource             plan.ResourceChange
	Attributes           []string
	Metadata             []string
	UnknownPaths         []string
	DefaultPaths         []string
	OmittedComputedPaths []string
	SummarizedPaths      []string
	OmittedAttributes    int
}

// RenderCompact emits TFP2. Review mode is the default; Detail retains the
// full resource-scoped record form for troubleshooting.
func RenderCompact(p *plan.Plan, opts Options) string {
	if opts.Detail {
		return renderCompactDetail(p, opts)
	}
	return renderCompactReview(p, opts)
}

// renderCompactDetail emits the original full resource-scoped TFP2 form.
func renderCompactDetail(p *plan.Plan, opts Options) string {
	var b strings.Builder
	opts.Limits = exactLimits()
	records := compactRecords(selectedResources(p, opts), opts)
	b.WriteString(compactHeaderLine(p, opts))
	b.WriteByte('\n')
	if opts.HeaderOnly {
		return b.String()
	}

	if !opts.NoGroups {
		for _, line := range typeSummaryLines(records) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		for _, line := range reasonSummaryLines(records) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if p.DriftSummary.Total > 0 {
		fmt.Fprintf(&b, "DRIFT|total=%d;types=%s;risk=%d\n", p.DriftSummary.Total, joinEscaped(p.DriftSummary.Types), p.DriftSummary.RiskResources)
	}
	if p.Metadata.CheckCount > 0 || p.Metadata.RelevantAttributeCount > 0 || p.Metadata.GeneratedConfigCount > 0 || p.Metadata.ImportCount > 0 {
		fmt.Fprintf(&b, "META|checks=%d;check_fail=%d;imports=%d;generated_config=%d;relevant_attrs=%d\n",
			p.Metadata.CheckCount,
			p.Metadata.FailedCheckCount,
			p.Metadata.ImportCount,
			p.Metadata.GeneratedConfigCount,
			p.Metadata.RelevantAttributeCount,
		)
	}

	for _, record := range records {
		fmt.Fprintf(&b, "%s|%s|%s|%s\n",
			record.Resource.Action,
			codec.Escape(record.Resource.Address),
			strings.Join(record.Attributes, ";"),
			strings.Join(record.Metadata, ";"),
		)
	}
	if opts.IncludeNoOp && opts.Summary {
		for _, resource := range p.NoOpResources {
			record := compactResource(resource, opts)
			fmt.Fprintf(&b, "%s|%s|%s|%s\n", resource.Action, codec.Escape(resource.Address), strings.Join(record.Attributes, ";"), strings.Join(record.Metadata, ";"))
		}
	}

	if !opts.RiskOnly {
		for _, output := range p.Outputs {
			expr, _ := renderCompactOutput(output, opts.Limits)
			meta := compactOutputMetadata(output, false)
			fmt.Fprintf(&b, "%s|%s|%s|%s\n", plan.ActionOutput, compactOutputName(output), expr, strings.Join(meta, ";"))
		}
	}

	for _, drift := range p.Drift {
		if !shouldEmitDriftDetail(drift, p.Resources) {
			continue
		}
		record := compactResource(drift, opts)
		fmt.Fprintf(&b, "DRIFT|%s|%s|%s|%s\n",
			drift.Action,
			codec.Escape(drift.Address),
			strings.Join(record.Attributes, ";"),
			strings.Join(record.Metadata, ";"),
		)
	}
	return b.String()
}

// CompactDetailCount reports how many attribute details survive TFP2 compression.
func CompactDetailCount(p *plan.Plan, opts Options) int {
	count := 0
	for _, record := range compactRecords(selectedResources(p, opts), opts) {
		count += len(record.Attributes)
	}
	if !opts.RiskOnly {
		for _, output := range p.Outputs {
			count += len(output.Attributes)
		}
	}
	return count
}

func compactRecords(resources []plan.ResourceChange, opts Options) []compactResourceRecord {
	records := make([]compactResourceRecord, 0, len(resources))
	for _, resource := range resources {
		records = append(records, compactResource(resource, opts))
	}
	return records
}

func compactResource(resource plan.ResourceChange, opts Options) compactResourceRecord {
	record := compactResourceRecord{Resource: resource, UnknownPaths: append([]string(nil), resource.UnknownPaths...)}
	for _, attribute := range resource.Attributes {
		if opts.Summary || (opts.MetadataOnly && len(resource.Risks) == 0 && !mustRetainCompactAttribute(resource, attribute)) || (!opts.Detail && shouldOmitCompactAttribute(resource, attribute)) {
			record.OmittedAttributes++
			if shouldCollapseDefault(resource, attribute) {
				record.DefaultPaths = append(record.DefaultPaths, attribute.Path)
			} else if shouldCollapseComputed(resource, attribute) {
				record.OmittedComputedPaths = append(record.OmittedComputedPaths, attribute.Path)
			}
			continue
		}
		rendered, summarized := renderCompactAttributeWithSummary(resource.Action, attribute, opts.Limits)
		record.Attributes = append(record.Attributes, rendered)
		if summarized {
			record.SummarizedPaths = append(record.SummarizedPaths, attribute.Path)
		}
	}
	record.DefaultPaths = uniqueSortedStrings(record.DefaultPaths)
	record.OmittedComputedPaths = uniqueSortedStrings(record.OmittedComputedPaths)
	record.SummarizedPaths = uniqueSortedStrings(record.SummarizedPaths)
	record.Metadata = compactResourceMetadata(resource, record)
	return record
}

func mustRetainCompactAttribute(resource plan.ResourceChange, attribute plan.AttributeChange) bool {
	return resource.Action == plan.ActionUpdate ||
		resource.Action == plan.ActionReplace ||
		attribute.ReplacePath ||
		attribute.Sensitive ||
		len(resource.Risks) > 0
}

func shouldOmitCompactAttribute(resource plan.ResourceChange, attribute plan.AttributeChange) bool {
	if len(resource.Risks) > 0 || attribute.ReplacePath || attribute.Sensitive {
		return false
	}
	if shouldCollapseDefault(resource, attribute) {
		return true
	}
	if shouldCollapseComputed(resource, attribute) {
		return true
	}
	return false
}

func shouldCollapseComputed(resource plan.ResourceChange, attribute plan.AttributeChange) bool {
	if attribute.ReplacePath || attribute.Sensitive || len(resource.Risks) > 0 {
		return false
	}
	if !isNoisyComputedPath(attribute.Path) {
		return false
	}
	if attribute.AfterUnknown {
		return resource.Action == plan.ActionCreate || hasSemanticAttribute(resource)
	}
	return hasSemanticAttribute(resource)
}

func hasSemanticAttribute(resource plan.ResourceChange) bool {
	for _, attribute := range resource.Attributes {
		if !isNoisyComputedPath(attribute.Path) && !shouldCollapseDefault(resource, attribute) {
			return true
		}
	}
	return false
}

func shouldCollapseDefault(resource plan.ResourceChange, attribute plan.AttributeChange) bool {
	if attribute.ReplacePath || attribute.Sensitive || len(resource.Risks) > 0 {
		return false
	}
	if _, ok := defaultTopLevelFields[topLevelPath(attribute.Path)]; !ok {
		return false
	}
	switch resource.Action {
	case plan.ActionCreate:
		return isEmptyDefaultValue(attribute.After)
	case plan.ActionDelete:
		return isEmptyDefaultValue(attribute.Before)
	default:
		return false
	}
}

func isNoisyComputedPath(path string) bool {
	_, ok := noisyComputedTopLevelFields[topLevelPath(path)]
	return ok
}

func topLevelPath(path string) string {
	for i, r := range path {
		if r == '.' || r == '[' {
			return path[:i]
		}
	}
	return path
}

func isEmptyDefaultValue(value plan.Value) bool {
	if value.Kind == plan.ValueNull {
		return true
	}
	if value.Kind != plan.ValueRaw {
		return false
	}
	switch typed := value.Raw.(type) {
	case map[string]any:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	}
	return false
}

func compactResourceMetadata(resource plan.ResourceChange, record compactResourceRecord) []string {
	var meta []string
	if len(record.Attributes) == 0 {
		meta = append(meta, "type="+codec.Escape(resource.Type))
	}
	if resource.Mode != "" && resource.Mode != "managed" {
		meta = append(meta, "mode="+codec.Escape(resource.Mode))
	}
	if resource.Action == plan.ActionReplace || resource.Action == plan.ActionRead {
		meta = append(meta, "actions="+joinEscaped(resource.RawActions))
	}
	if resource.ActionReason != "" {
		meta = append(meta, "reason="+codec.Escape(resource.ActionReason))
	}
	if len(resource.UnknownPaths) > 0 {
		meta = append(meta, "unknown="+joinEscaped(resource.UnknownPaths))
	}
	if len(resource.SensitivePaths) > 0 {
		meta = append(meta, "sensitive="+joinEscaped(resource.SensitivePaths))
	}
	if len(resource.ReplacePaths) > 0 {
		meta = append(meta, "replace_path="+joinEscaped(resource.ReplacePaths))
	}
	if risks := riskNames(resource); len(risks) > 0 {
		meta = append(meta, "risk="+joinEscaped(risks))
	}
	if len(record.DefaultPaths) > 0 {
		meta = append(meta, "defaults="+joinEscaped(record.DefaultPaths))
	}
	if resource.PreviousAddress != "" {
		meta = append(meta, "previous="+codec.Escape(resource.PreviousAddress))
	}
	if resource.Deposed != "" {
		meta = append(meta, "deposed="+codec.Escape(resource.Deposed))
	}
	if resource.Importing {
		meta = append(meta, "import=true")
	}
	if resource.GeneratedConfig {
		meta = append(meta, "generated_config=true")
	}
	if record.OmittedAttributes > 0 {
		meta = append(meta, fmt.Sprintf("omitted_attrs=%d", record.OmittedAttributes))
	}
	if len(record.SummarizedPaths) > 0 {
		meta = append(meta, "summarized="+joinEscaped(record.SummarizedPaths))
		meta = append(meta, "detail_required=true")
	}
	if len(record.Attributes) == 0 {
		meta = append(meta, "attrs=none")
		if len(resource.Attributes) == 0 {
			meta = append(meta, "no_material_attrs=true")
		}
		if resource.ProviderName != "" {
			meta = append(meta, "provider="+codec.Escape(resource.ProviderName))
		}
	}
	return meta
}

func compactOutputMetadata(output plan.OutputChange, summarized bool) []string {
	var meta []string
	if len(output.UnknownPaths) > 0 {
		meta = append(meta, "unknown="+joinEscaped(output.UnknownPaths))
	}
	if len(output.SensitivePaths) > 0 {
		meta = append(meta, "sensitive="+joinEscaped(output.SensitivePaths))
	}
	if summarized {
		meta = append(meta, "summary=true", "detail_required=true")
	}
	return meta
}

func renderCompactAttribute(action plan.Action, attribute plan.AttributeChange, limits Limits) string {
	rendered, _ := renderCompactAttributeWithSummary(action, attribute, limits)
	return rendered
}

func renderCompactAttributeWithSummary(action plan.Action, attribute plan.AttributeChange, limits Limits) (string, bool) {
	path := codec.Escape(attribute.Path)
	switch action {
	case plan.ActionCreate:
		after, afterSummary := lineValueWithSummary(attribute.After, limits)
		return path + "=" + codec.Escape(after), afterSummary
	case plan.ActionDelete:
		before, beforeSummary := lineValueWithSummary(attribute.Before, limits)
		return path + "=" + codec.Escape(before), beforeSummary
	default:
		before, beforeSummary := lineValueWithSummary(attribute.Before, limits)
		after, afterSummary := lineValueWithSummary(attribute.After, limits)
		return path + "=" + codec.Escape(before) + "->" + codec.Escape(after), beforeSummary || afterSummary
	}
}

func renderCompactOutput(output plan.OutputChange, limits Limits) (string, bool) {
	if len(output.Attributes) == 0 {
		return "attrs=none", false
	}
	attribute := output.Attributes[0]
	before, beforeSummary := lineValueWithSummary(attribute.Before, limits)
	after, afterSummary := lineValueWithSummary(attribute.After, limits)
	switch output.Action {
	case plan.ActionCreate:
		return "+" + codec.Escape(after), afterSummary
	case plan.ActionDelete:
		return "-" + codec.Escape(before), beforeSummary
	default:
		return codec.Escape(before) + "->" + codec.Escape(after), beforeSummary || afterSummary
	}
}

func compactOutputName(output plan.OutputChange) string {
	if output.Name != "" {
		return codec.Escape(output.Name)
	}
	return codec.Escape(strings.TrimPrefix(output.Address, "output."))
}

func exactLimits() Limits {
	return Limits{MaxValueLen: -1, MaxListItems: -1, MaxObjectKeys: -1}
}

func typeSummaryLines(records []compactResourceRecord) []string {
	type group struct {
		Type     string
		Counts   map[plan.Action]int
		Unknowns []string
		Defaults []string
		Provider string
		Total    int
	}
	groups := map[string]*group{}
	for _, record := range records {
		key := record.Resource.Type
		g := groups[key]
		if g == nil {
			g = &group{Type: key, Counts: make(map[plan.Action]int), Unknowns: append([]string(nil), record.Resource.UnknownPaths...), Defaults: append([]string(nil), record.DefaultPaths...), Provider: record.Resource.ProviderName}
			groups[key] = g
		} else {
			g.Unknowns = unionStrings(g.Unknowns, record.Resource.UnknownPaths)
			g.Defaults = intersectStrings(g.Defaults, record.DefaultPaths)
			if g.Provider != record.Resource.ProviderName {
				g.Provider = ""
			}
		}
		g.Counts[record.Resource.Action]++
		g.Total++
	}
	keys := make([]string, 0, len(groups))
	for key, group := range groups {
		if group.Total >= 3 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var lines []string
	for _, key := range keys {
		g := groups[key]
		counts := compactActionCounts(g.Counts)
		var meta []string
		if len(g.Unknowns) > 0 {
			meta = append(meta, "unknown="+joinEscaped(g.Unknowns))
		}
		if len(g.Defaults) > 0 {
			meta = append(meta, "defaults="+joinEscaped(g.Defaults))
		}
		if g.Provider != "" {
			meta = append(meta, "provider="+codec.Escape(g.Provider))
		}
		lines = append(lines, fmt.Sprintf("TYPE|%s|%s|%s", codec.Escape(g.Type), counts, strings.Join(meta, ";")))
	}
	return lines
}

func reasonSummaryLines(records []compactResourceRecord) []string {
	counts := make(map[string]int)
	for _, record := range records {
		if record.Resource.ActionReason != "" {
			counts[record.Resource.ActionReason]++
		}
	}
	keys := make([]string, 0, len(counts))
	for reason, count := range counts {
		if count >= 2 {
			keys = append(keys, reason)
		}
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, reason := range keys {
		lines = append(lines, fmt.Sprintf("REASON|%s|COUNT=%d", codec.Escape(reason), counts[reason]))
	}
	return lines
}

func compactActionCounts(counts map[plan.Action]int) string {
	order := []plan.Action{plan.ActionCreate, plan.ActionUpdate, plan.ActionReplace, plan.ActionDelete, plan.ActionRead}
	var parts []string
	for _, action := range order {
		if counts[action] > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", action, counts[action]))
		}
	}
	return strings.Join(parts, ";")
}

func compactHeaderLine(p *plan.Plan, opts Options) string {
	header := fmt.Sprintf("TFP2 C=%d U=%d R=%d D=%d Q=%d OUT=%d RISK=%d DRIFT=%d", p.Summary.Creates, p.Summary.Updates, p.Summary.Replaces, p.Summary.Deletes, p.Summary.Reads, p.Summary.OutputChanges, p.Summary.RiskResources, p.DriftSummary.Total)
	if opts.Omitted > 0 {
		header += fmt.Sprintf(" OMITTED=%d", opts.Omitted)
	}
	return header
}

func shouldEmitDriftDetail(drift plan.ResourceChange, planned []plan.ResourceChange) bool {
	if len(drift.Risks) > 0 || hasIdentityOrSecurityPath(drift.Attributes) {
		return true
	}
	for _, resource := range planned {
		if resource.Address == drift.Address {
			return true
		}
	}
	return false
}

func hasIdentityOrSecurityPath(attributes []plan.AttributeChange) bool {
	for _, attribute := range attributes {
		path := strings.ToLower(attribute.Path)
		for _, term := range []string{"id", "name", "arn", "ingress", "egress", "cidr", "policy", "iam", "security", "network", "principal", "member", "role"} {
			if strings.Contains(path, term) {
				return true
			}
		}
	}
	return false
}

func riskNames(resource plan.ResourceChange) []string {
	values := make([]string, 0, len(resource.Risks))
	for _, risk := range resource.Risks {
		values = append(values, risk.Name)
	}
	return values
}

func joinEscaped(values []string) string {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		encoded = append(encoded, codec.EscapeListItem(value))
	}
	return strings.Join(encoded, ",")
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func intersectStrings(left, right []string) []string {
	seen := make(map[string]struct{}, len(right))
	for _, value := range right {
		seen[value] = struct{}{}
	}
	var out []string
	for _, value := range left {
		if _, ok := seen[value]; ok {
			out = append(out, value)
		}
	}
	return uniqueSortedStrings(out)
}

func unionStrings(left, right []string) []string {
	return uniqueSortedStrings(append(append([]string(nil), left...), right...))
}
