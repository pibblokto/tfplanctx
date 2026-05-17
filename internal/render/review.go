package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pibblokto/tfplanctx/internal/codec"
	"github.com/pibblokto/tfplanctx/internal/plan"
)

const (
	minGroupSavingsRatio = 0.10
	minGroupSavingsChars = 200
	minPairSavingsChars  = 150
	maxAddressTemplates  = 50
	maxValueDictionaries = 100
)

var compactReasonCodes = map[string]string{
	"delete_because_no_resource_config": "no_config",
	"replace_because_cannot_update":     "cannot_update",
	"replace_because_tainted":           "tainted",
	"replace_by_request":                "requested",
	"delete_because_wrong_repetition":   "wrong_repetition",
	"delete_because_count_index":        "count_index",
	"delete_because_each_key":           "each_key",
	"read_because_config_unknown":       "config_unknown",
}

// CompactStats exposes deterministic review-mode compression facts for benchmarks.
type CompactStats struct {
	Omitted             int
	OmittedComputed     int
	OmittedDefaultEmpty int
	OmittedBudget       int
	SummarizedValues    int
	GroupedCommon       int
	GroupCount          int
	GroupedResources    int
	TemplateCount       int
	DictionaryCount     int
	LensResources       int
	DriftSummarized     int
	DriftDetailed       int
}

type reviewAttribute struct {
	Path  string
	Value string
}

type reviewRecord struct {
	compactResourceRecord
	Attrs []reviewAttribute
}

type reviewGroup struct {
	ID          string
	Action      plan.Action
	Type        string
	Records     []reviewRecord
	CommonAttrs []reviewAttribute
	Columns     []string
	Meta        []string
	ListColumn  string
	ListValues  []string
}

type addressTemplate struct {
	ID     string
	Prefix string
	Saving int
}

type valueDictionary struct {
	ID     string
	Value  string
	Saving int
}

type iamLens struct {
	ID             string
	Action         plan.Action
	Type           string
	Records        []reviewRecord
	ScopePath      string
	ScopeValue     string
	PrincipalPath  string
	PrincipalValue string
	RolePath       string
	Roles          []string
	Meta           []string
}

type migrationSummary struct {
	Type        string
	Creates     int
	Deletes     int
	SameScope   string
	CommonRoles []string
}

type reviewDriftGroup struct {
	Type   string
	Count  int
	Fields []string
	Class  string
}

type reviewModel struct {
	Records           []reviewRecord
	Groups            []reviewGroup
	Lenses            []iamLens
	Ungrouped         []reviewRecord
	Templates         []addressTemplate
	Dictionaries      []valueDictionary
	Migrations        []migrationSummary
	ReasonLegend      map[string]string // code -> full reason
	DriftGroups       []reviewDriftGroup
	DetailedDrift     []compactResourceRecord
	Stats             CompactStats
	OmissionFields    []string
	CompressionFields []string
}

func renderCompactReview(p *plan.Plan, opts Options) string {
	model := buildReviewModel(p, opts)
	var b strings.Builder
	b.WriteString(compactReviewHeaderLine(p, opts, model.Stats))
	b.WriteByte('\n')
	if opts.HeaderOnly {
		return b.String()
	}

	if len(model.OmissionFields) > 0 {
		fmt.Fprintf(&b, "OMIT|%s\n", strings.Join(model.OmissionFields, ";"))
	}
	if len(model.CompressionFields) > 0 {
		fmt.Fprintf(&b, "COMPRESS|%s\n", strings.Join(model.CompressionFields, ";"))
	}
	for _, template := range model.Templates {
		fmt.Fprintf(&b, "TPL|%s|%s\n", template.ID, codec.Escape(template.Prefix))
	}
	for _, dictionary := range model.Dictionaries {
		fmt.Fprintf(&b, "VAL|%s|%s\n", dictionary.ID, codec.Escape(dictionary.Value))
	}
	if len(model.ReasonLegend) > 0 {
		codes := make([]string, 0, len(model.ReasonLegend))
		for code := range model.ReasonLegend {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		parts := make([]string, 0, len(codes))
		for _, code := range codes {
			parts = append(parts, codec.Escape(code)+"="+codec.Escape(model.ReasonLegend[code]))
		}
		fmt.Fprintf(&b, "REASON_CODES|%s\n", strings.Join(parts, ";"))
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
	for _, migration := range model.Migrations {
		fields := []string{
			"type=" + codec.Escape(migration.Type),
			fmt.Sprintf("C=%d", migration.Creates),
			fmt.Sprintf("D=%d", migration.Deletes),
		}
		if migration.SameScope != "" {
			fields = append(fields, "same_scope="+renderDictionaryValue(migration.SameScope, model.Dictionaries))
		}
		if len(migration.CommonRoles) > 0 {
			fields = append(fields, "common_roles="+renderDictionaryList(migration.CommonRoles, model.Dictionaries))
		}
		fields = append(fields, "confidence=structural")
		fmt.Fprintf(&b, "MIGRATION?|%s\n", strings.Join(fields, ";"))
	}
	for _, lens := range model.Lenses {
		b.WriteString(renderIAMLensLine(lens, model.Templates, model.Dictionaries))
	}

	for _, group := range model.Groups {
		renderReviewGroup(&b, group, model.Templates, model.Dictionaries)
	}
	for _, record := range model.Ungrouped {
		fmt.Fprintf(&b, "%s|%s|%s|%s\n",
			record.Resource.Action,
			renderTemplatedAddress(record.Resource.Address, model.Templates),
			renderReviewAttrs(record.Attrs, model.Dictionaries),
			strings.Join(reviewMetadata(record, model.ReasonLegend), ";"),
		)
	}
	if opts.IncludeNoOp && opts.Summary {
		for _, resource := range p.NoOpResources {
			record := reviewRecordFromCompact(compactResource(resource, opts))
			fmt.Fprintf(&b, "%s|%s|%s|%s\n",
				resource.Action,
				renderTemplatedAddress(resource.Address, model.Templates),
				renderReviewAttrs(record.Attrs, model.Dictionaries),
				strings.Join(reviewMetadata(record, model.ReasonLegend), ";"),
			)
		}
	}
	if !opts.RiskOnly {
		for _, output := range p.Outputs {
			expr, summarized := renderCompactOutput(output, opts.Limits)
			fmt.Fprintf(&b, "%s|%s|%s|%s\n",
				plan.ActionOutput,
				compactOutputName(output),
				expr,
				strings.Join(reviewOutputMetadata(output, summarized), ";"),
			)
		}
	}
	if p.DriftSummary.Total > 0 {
		fmt.Fprintf(&b, "DRIFT|total=%d;risk=%d;summ=%d;detail=%d\n",
			p.DriftSummary.Total,
			p.DriftSummary.RiskResources,
			model.Stats.DriftSummarized,
			model.Stats.DriftDetailed,
		)
		for _, group := range model.DriftGroups {
			fmt.Fprintf(&b, "DRIFT_GROUP|type=%s;count=%d;fields=%s;class=%s\n",
				codec.Escape(group.Type),
				group.Count,
				joinEscaped(group.Fields),
				group.Class,
			)
		}
		for _, drift := range model.DetailedDrift {
			fmt.Fprintf(&b, "DRIFT_DETAIL|%s|%s|%s|%s\n",
				drift.Resource.Action,
				codec.Escape(drift.Resource.Address),
				strings.Join(drift.Attributes, ";"),
				strings.Join(reviewMetadata(reviewRecordFromCompact(drift), model.ReasonLegend), ";"),
			)
		}
	}
	return b.String()
}

// CompactReviewStats returns the same stats used by review rendering.
func CompactReviewStats(p *plan.Plan, opts Options) CompactStats {
	return buildReviewModel(p, opts).Stats
}

func buildReviewModel(p *plan.Plan, opts Options) reviewModel {
	records := reviewRecords(selectedResources(p, opts), opts)
	model := reviewModel{
		Records:      records,
		Templates:    selectAddressTemplates(records),
		Dictionaries: selectValueDictionaries(records),
		ReasonLegend: selectReasonLegend(records),
	}
	var remaining []reviewRecord
	model.Lenses, remaining = selectIAMLenses(records, model.Templates, model.Dictionaries, model.ReasonLegend)
	model.Groups, model.Ungrouped, model.Stats.GroupedCommon = selectReviewGroups(remaining, model.Templates, model.Dictionaries, model.ReasonLegend)
	model.Migrations = selectMigrationSummaries(records)
	model.Stats.GroupCount = len(model.Groups)
	for _, group := range model.Groups {
		model.Stats.GroupedResources += len(group.Records)
	}
	model.Stats.TemplateCount = len(model.Templates)
	model.Stats.DictionaryCount = len(model.Dictionaries)
	for _, lens := range model.Lenses {
		model.Stats.LensResources += len(lens.Records)
	}
	for _, record := range records {
		model.Stats.OmittedComputed += len(record.OmittedComputedPaths)
		model.Stats.OmittedDefaultEmpty += len(record.DefaultPaths)
		model.Stats.SummarizedValues += len(record.SummarizedPaths)
	}
	for _, output := range p.Outputs {
		_, summarized := renderCompactOutput(output, opts.Limits)
		if summarized {
			model.Stats.SummarizedValues++
		}
	}
	model.Stats.OmittedBudget = opts.Omitted
	model.DriftGroups, model.DetailedDrift = classifyPlanDrift(p.Drift, p.Resources, opts)
	model.Stats.DriftSummarized = 0
	for _, group := range model.DriftGroups {
		model.Stats.DriftSummarized += group.Count
	}
	model.Stats.DriftDetailed = len(model.DetailedDrift)
	model.Stats.Omitted = model.Stats.OmittedComputed + model.Stats.OmittedDefaultEmpty + model.Stats.DriftSummarized + model.Stats.OmittedBudget + model.Stats.SummarizedValues
	model.OmissionFields = omissionFields(model.Stats)
	model.CompressionFields = compressionFields(model.Stats)
	return model
}

func reviewRecords(resources []plan.ResourceChange, opts Options) []reviewRecord {
	compact := compactRecords(resources, opts)
	records := make([]reviewRecord, 0, len(compact))
	for _, record := range compact {
		records = append(records, reviewRecordFromCompact(record))
	}
	return records
}

func reviewRecordFromCompact(record compactResourceRecord) reviewRecord {
	review := reviewRecord{compactResourceRecord: record}
	for _, attribute := range record.Attributes {
		path, value, ok := splitRenderedAttribute(attribute)
		if !ok {
			continue
		}
		review.Attrs = append(review.Attrs, reviewAttribute{Path: path, Value: value})
	}
	return review
}

func splitRenderedAttribute(attribute string) (string, string, bool) {
	parts := strings.SplitN(attribute, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	path, err := codec.Unescape(parts[0])
	if err != nil {
		path = parts[0]
	}
	return path, parts[1], true
}

func selectReviewGroups(records []reviewRecord, templates []addressTemplate, dictionaries []valueDictionary, legend map[string]string) ([]reviewGroup, []reviewRecord, int) {
	buckets := make(map[string][]reviewRecord)
	for _, record := range records {
		if !isGroupable(record) {
			continue
		}
		buckets[groupKey(record, legend)] = append(buckets[groupKey(record, legend)], record)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	groupedAddresses := make(map[string]struct{})
	var groups []reviewGroup
	groupedCommon := 0
	for _, key := range keys {
		candidateRecords := buckets[key]
		if len(candidateRecords) < 2 {
			continue
		}
		group := buildReviewGroup(candidateRecords, fmt.Sprintf("G%d", len(groups)+1), legend)
		ungroupedCost := 0
		for _, record := range candidateRecords {
			ungroupedCost += len(renderReviewRecordLine(record, templates, dictionaries, legend))
		}
		group = maybeListCompressGroup(group, templates, dictionaries)
		groupedCost := estimateReviewGroupCost(group, templates, dictionaries)
		saving := ungroupedCost - groupedCost
		ratio := 0.0
		if ungroupedCost > 0 {
			ratio = float64(saving) / float64(ungroupedCost)
		}
		use := false
		switch {
		case len(candidateRecords) >= 3:
			use = saving > 0 && (ratio >= minGroupSavingsRatio || saving >= minGroupSavingsChars)
		case len(candidateRecords) == 2:
			use = saving >= minPairSavingsChars
		}
		if !use {
			continue
		}
		groups = append(groups, group)
		groupedCommon += len(group.CommonAttrs) * len(group.Records)
		for _, record := range candidateRecords {
			groupedAddresses[record.Resource.Address] = struct{}{}
		}
	}
	var ungrouped []reviewRecord
	for _, record := range records {
		if _, ok := groupedAddresses[record.Resource.Address]; !ok {
			ungrouped = append(ungrouped, record)
		}
	}
	return groups, ungrouped, groupedCommon
}

func isGroupable(record reviewRecord) bool {
	resource := record.Resource
	return (resource.Mode == "" || resource.Mode == "managed") &&
		resource.PreviousAddress == "" &&
		resource.Deposed == "" &&
		!resource.Importing &&
		!resource.GeneratedConfig
}

func groupKey(record reviewRecord, legend map[string]string) string {
	resource := record.Resource
	parts := []string{
		string(resource.Action),
		resource.Type,
		resource.ProviderName,
		reviewReason(resource.ActionReason, legend),
		strings.Join(reviewAttrPaths(record.Attrs), ","),
		strings.Join(resource.UnknownPaths, ","),
		strings.Join(resource.SensitivePaths, ","),
		strings.Join(record.DefaultPaths, ","),
		strings.Join(record.OmittedComputedPaths, ","),
		strings.Join(resource.ReplacePaths, ","),
		strings.Join(riskNames(resource), ","),
		strings.Join(resource.RawActions, ","),
		fmt.Sprintf("%d", record.OmittedAttributes),
	}
	return strings.Join(parts, "\x00")
}

func buildReviewGroup(records []reviewRecord, id string, legend map[string]string) reviewGroup {
	sort.Slice(records, func(i, j int) bool { return records[i].Resource.Address < records[j].Resource.Address })
	first := records[0]
	group := reviewGroup{ID: id, Action: first.Resource.Action, Type: first.Resource.Type, Records: records}
	for _, attr := range first.Attrs {
		value := attr.Value
		common := true
		for _, record := range records[1:] {
			if reviewAttrValue(record.Attrs, attr.Path) != value {
				common = false
				break
			}
		}
		if common {
			group.CommonAttrs = append(group.CommonAttrs, attr)
		}
	}
	commonPaths := make(map[string]struct{}, len(group.CommonAttrs))
	for _, attr := range group.CommonAttrs {
		commonPaths[attr.Path] = struct{}{}
	}
	group.Columns = append(group.Columns, "addr")
	for _, attr := range first.Attrs {
		if _, ok := commonPaths[attr.Path]; !ok {
			group.Columns = append(group.Columns, attr.Path)
		}
	}
	group.Meta = reviewMetadata(first, legend)
	return group
}

func estimateReviewGroupCost(group reviewGroup, templates []addressTemplate, dictionaries []valueDictionary) int {
	if group.ListColumn != "" {
		return len(renderListGroupLine(group, templates, dictionaries))
	}
	cost := len(fmt.Sprintf("G|%s|%s|%s|n=%d;cols=%s",
		group.ID,
		group.Action,
		codec.Escape(group.Type),
		len(group.Records),
		joinEscaped(group.Columns),
	))
	if len(group.CommonAttrs) > 0 {
		cost += len(";common=") + len(renderCommonAttrs(group.CommonAttrs, dictionaries))
	}
	if len(group.Meta) > 0 {
		cost += 1 + len(strings.Join(group.Meta, ";"))
	}
	cost++ // newline
	for _, record := range group.Records {
		values := []string{renderTemplatedAddress(record.Resource.Address, templates)}
		for _, column := range group.Columns[1:] {
			values = append(values, renderDictionaryValue(reviewAttrValue(record.Attrs, column), dictionaries))
		}
		cost += 1 + len(strings.Join(values, "|")) + 1
	}
	return cost
}

func renderReviewRecordLine(record reviewRecord, templates []addressTemplate, dictionaries []valueDictionary, legend map[string]string) string {
	return fmt.Sprintf("%s|%s|%s|%s\n",
		record.Resource.Action,
		renderTemplatedAddress(record.Resource.Address, templates),
		renderReviewAttrs(record.Attrs, dictionaries),
		strings.Join(reviewMetadata(record, legend), ";"),
	)
}

func renderReviewAttrs(attrs []reviewAttribute, dictionaries []valueDictionary) string {
	parts := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		parts = append(parts, codec.Escape(attr.Path)+"="+renderDictionaryValue(attr.Value, dictionaries))
	}
	return strings.Join(parts, ";")
}

func renderCommonAttrs(attrs []reviewAttribute, dictionaries []valueDictionary) string {
	parts := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		parts = append(parts, codec.EscapeListItem(attr.Path)+":"+renderDictionaryValue(attr.Value, dictionaries))
	}
	return strings.Join(parts, ",")
}

func reviewAttrPaths(attrs []reviewAttribute) []string {
	paths := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		paths = append(paths, attr.Path)
	}
	return paths
}

func reviewAttrValue(attrs []reviewAttribute, path string) string {
	for _, attr := range attrs {
		if attr.Path == path {
			return attr.Value
		}
	}
	return ""
}

func reviewMetadata(record reviewRecord, legend map[string]string) []string {
	resource := record.Resource
	var meta []string
	if len(record.Attrs) == 0 {
		meta = append(meta, "type="+codec.Escape(resource.Type))
	}
	if resource.Mode != "" && resource.Mode != "managed" {
		meta = append(meta, "mode="+codec.Escape(resource.Mode))
	}
	if resource.Action == plan.ActionReplace || resource.Action == plan.ActionRead {
		meta = append(meta, "acts="+joinEscaped(resource.RawActions))
	}
	if resource.ActionReason != "" {
		meta = append(meta, "why="+codec.Escape(reviewReason(resource.ActionReason, legend)))
	}
	if len(resource.UnknownPaths) > 0 {
		meta = append(meta, "unk="+joinEscaped(resource.UnknownPaths))
	}
	if len(resource.SensitivePaths) > 0 {
		meta = append(meta, "sens="+joinEscaped(resource.SensitivePaths))
	}
	if len(resource.ReplacePaths) > 0 {
		meta = append(meta, "repl="+joinEscaped(resource.ReplacePaths))
	}
	if risks := riskNames(resource); len(risks) > 0 {
		meta = append(meta, "risk="+joinEscaped(risks))
	}
	if len(record.DefaultPaths) > 0 {
		meta = append(meta, "def="+joinEscaped(record.DefaultPaths))
	}
	if len(record.OmittedComputedPaths) > 0 {
		meta = append(meta, "comp="+joinEscaped(record.OmittedComputedPaths))
	}
	if len(record.SummarizedPaths) > 0 {
		meta = append(meta, "summ="+joinEscaped(record.SummarizedPaths), "detail_required=true")
	}
	if resource.PreviousAddress != "" {
		meta = append(meta, "prev="+codec.Escape(resource.PreviousAddress))
	}
	if resource.Deposed != "" {
		meta = append(meta, "deposed="+codec.Escape(resource.Deposed))
	}
	if resource.Importing {
		meta = append(meta, "import=true")
	}
	if resource.GeneratedConfig {
		meta = append(meta, "gen=true")
	}
	if record.OmittedAttributes > 0 {
		meta = append(meta, fmt.Sprintf("omit=%d", record.OmittedAttributes))
	}
	if len(record.Attrs) == 0 {
		meta = append(meta, "attrs=none")
		if len(resource.Attributes) == 0 {
			meta = append(meta, "no_material_attrs=true")
		}
	}
	return meta
}

func reviewOutputMetadata(output plan.OutputChange, summarized bool) []string {
	var meta []string
	if len(output.UnknownPaths) > 0 {
		meta = append(meta, "unk="+joinEscaped(output.UnknownPaths))
	}
	if len(output.SensitivePaths) > 0 {
		meta = append(meta, "sens="+joinEscaped(output.SensitivePaths))
	}
	if summarized {
		meta = append(meta, "summary=true", "detail_required=true")
	}
	return meta
}

func selectReasonLegend(records []reviewRecord) map[string]string {
	counts := make(map[string]int)
	for _, record := range records {
		if record.Resource.ActionReason != "" {
			counts[record.Resource.ActionReason]++
		}
	}
	legend := make(map[string]string)
	for full, code := range compactReasonCodes {
		if counts[full] >= 2 {
			legend[code] = full
		}
	}
	return legend
}

func reviewReason(reason string, legend map[string]string) string {
	if code, ok := compactReasonCodes[reason]; ok {
		if _, enabled := legend[code]; enabled {
			return code
		}
	}
	return reason
}

func omissionFields(stats CompactStats) []string {
	var fields []string
	if stats.OmittedComputed > 0 {
		fields = append(fields, fmt.Sprintf("computed=%d", stats.OmittedComputed))
	}
	if stats.OmittedDefaultEmpty > 0 {
		fields = append(fields, fmt.Sprintf("default_empty=%d", stats.OmittedDefaultEmpty))
	}
	if stats.DriftSummarized > 0 {
		fields = append(fields, fmt.Sprintf("drift_low_signal=%d", stats.DriftSummarized))
	}
	if stats.OmittedBudget > 0 {
		fields = append(fields, fmt.Sprintf("budget=%d", stats.OmittedBudget))
	}
	if stats.SummarizedValues > 0 {
		fields = append(fields, fmt.Sprintf("summarized=%d", stats.SummarizedValues))
	}
	return fields
}

func compressionFields(stats CompactStats) []string {
	var fields []string
	if stats.GroupedCommon > 0 {
		fields = append(fields, fmt.Sprintf("grouped_common=%d", stats.GroupedCommon))
	}
	if stats.GroupCount > 0 {
		fields = append(fields, fmt.Sprintf("groups=%d", stats.GroupCount))
	}
	if stats.TemplateCount > 0 {
		fields = append(fields, fmt.Sprintf("templates=%d", stats.TemplateCount))
	}
	if stats.DictionaryCount > 0 {
		fields = append(fields, fmt.Sprintf("dict_values=%d", stats.DictionaryCount))
	}
	if stats.LensResources > 0 {
		fields = append(fields, fmt.Sprintf("lens_resources=%d", stats.LensResources))
	}
	return fields
}

func compactReviewHeaderLine(p *plan.Plan, opts Options, stats CompactStats) string {
	header := fmt.Sprintf("TFP2 C=%d U=%d R=%d D=%d Q=%d OUT=%d RISK=%d DRIFT=%d",
		p.Summary.Creates,
		p.Summary.Updates,
		p.Summary.Replaces,
		p.Summary.Deletes,
		p.Summary.Reads,
		p.Summary.OutputChanges,
		p.Summary.RiskResources,
		p.DriftSummary.Total,
	)
	if stats.Omitted > 0 {
		header += fmt.Sprintf(" OMITTED=%d", stats.Omitted)
	}
	return header
}

func selectAddressTemplates(records []reviewRecord) []addressTemplate {
	occurrences := make(map[string]int)
	for _, record := range records {
		if prefix := addressPrefix(record.Resource.Address); prefix != "" {
			occurrences[prefix]++
		}
	}
	var candidates []addressTemplate
	for prefix, count := range occurrences {
		if count < 2 {
			continue
		}
		definitionCost := len("TPL|P99|") + len(codec.Escape(prefix)) + 1
		untemplatedCost := count * len(codec.Escape(prefix))
		templatedCost := definitionCost + count*len("$P99:")
		saving := untemplatedCost - templatedCost
		ratio := 0.0
		if untemplatedCost > 0 {
			ratio = float64(saving) / float64(untemplatedCost)
		}
		if (count >= 3 && saving >= 100) || (count == 2 && saving >= 200) || ratio >= 0.15 {
			candidates = append(candidates, addressTemplate{Prefix: prefix, Saving: saving})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Saving != candidates[j].Saving {
			return candidates[i].Saving > candidates[j].Saving
		}
		return candidates[i].Prefix < candidates[j].Prefix
	})
	if len(candidates) > maxAddressTemplates {
		candidates = candidates[:maxAddressTemplates]
	}
	for index := range candidates {
		candidates[index].ID = fmt.Sprintf("P%d", index+1)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	return candidates
}

func addressPrefix(address string) string {
	lastDot, lastBracket := -1, -1
	depth := 0
	inQuote := false
	escaped := false
	for i := 0; i < len(address); i++ {
		ch := address[i]
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inQuote = false
			}
			continue
		}
		switch ch {
		case '"':
			if depth > 0 {
				inQuote = true
			}
		case '[':
			if depth == 0 {
				lastBracket = i
			}
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '.':
			if depth == 0 {
				lastDot = i
			}
		}
	}
	cut := lastDot
	if lastBracket > cut {
		cut = lastBracket
	}
	if cut < 0 {
		return ""
	}
	if cut == lastDot {
		return address[:cut+1]
	}
	return address[:cut]
}

func renderTemplatedAddress(address string, templates []addressTemplate) string {
	best := addressTemplate{}
	for _, template := range templates {
		if strings.HasPrefix(address, template.Prefix) && len(template.Prefix) > len(best.Prefix) {
			best = template
		}
	}
	if best.ID == "" {
		return codec.Escape(address)
	}
	return "$" + best.ID + ":" + codec.Escape(strings.TrimPrefix(address, best.Prefix))
}

func classifyPlanDrift(drift []plan.ResourceChange, planned []plan.ResourceChange, opts Options) ([]reviewDriftGroup, []compactResourceRecord) {
	grouped := make(map[string]*reviewDriftGroup)
	var details []compactResourceRecord
	for _, resource := range drift {
		class, fields, detailed := classifyDrift(resource, planned)
		if opts.Detail {
			detailed = true
		}
		if detailed {
			details = append(details, compactResource(resource, Options{Limits: opts.Limits, Detail: true}))
			continue
		}
		key := resource.Type + "\x00" + strings.Join(fields, ",") + "\x00" + class
		group := grouped[key]
		if group == nil {
			group = &reviewDriftGroup{Type: resource.Type, Fields: fields, Class: class}
			grouped[key] = group
		}
		group.Count++
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := make([]reviewDriftGroup, 0, len(keys))
	for _, key := range keys {
		groups = append(groups, *grouped[key])
	}
	return groups, details
}

func classifyDrift(resource plan.ResourceChange, planned []plan.ResourceChange) (string, []string, bool) {
	fields := driftTopLevelFields(resource.Attributes)
	if len(resource.Risks) > 0 {
		return "risk_relevant", fields, true
	}
	if len(resource.SensitivePaths) > 0 {
		return "risk_relevant", fields, true
	}
	for _, plannedResource := range planned {
		if plannedResource.Address == resource.Address {
			return "planned_change", fields, true
		}
	}
	class := classifyDriftFields(fields)
	switch class {
	case "computed_only", "provider_cache", "timestamp":
		return class, fields, false
	default:
		return class, fields, true
	}
}

func driftTopLevelFields(attributes []plan.AttributeChange) []string {
	fields := make([]string, 0, len(attributes))
	for _, attribute := range attributes {
		fields = append(fields, topLevelPath(attribute.Path))
	}
	return uniqueSortedStrings(fields)
}

func classifyDriftFields(fields []string) string {
	if len(fields) == 0 {
		return "unknown"
	}
	class := ""
	for _, field := range fields {
		next := classifyDriftField(field)
		if class == "" {
			class = next
			continue
		}
		if class != next {
			if isLowSignalDriftClass(class) && isLowSignalDriftClass(next) {
				class = "computed_only"
				continue
			}
			return "unknown"
		}
	}
	return class
}

func classifyDriftField(field string) string {
	lower := strings.ToLower(field)
	switch {
	case lower == "etag" || lower == "fingerprint" || strings.Contains(lower, "hash") || lower == "resource_version" || lower == "observed_generation":
		return "provider_cache"
	case strings.Contains(lower, "time") || strings.Contains(lower, "timestamp") || strings.Contains(lower, "last_modified"):
		return "timestamp"
	case lower == "id" || lower == "arn" || lower == "self_link" || lower == "uid" || lower == "name" || lower == "identifier":
		return "identity"
	case strings.Contains(lower, "ingress") || strings.Contains(lower, "egress") || strings.Contains(lower, "cidr") || strings.Contains(lower, "firewall") || strings.Contains(lower, "source_range"):
		return "network"
	case strings.Contains(lower, "security") || strings.Contains(lower, "privileged"):
		return "security"
	case strings.Contains(lower, "iam") || strings.Contains(lower, "policy") || strings.Contains(lower, "member") || strings.Contains(lower, "principal") || strings.Contains(lower, "role"):
		return "iam"
	case strings.Contains(lower, "lifecycle") || strings.Contains(lower, "retention"):
		return "lifecycle"
	case strings.Contains(lower, "acl") || strings.Contains(lower, "public_access") || strings.Contains(lower, "versioning"):
		return "storage_policy"
	case isNoisyComputedPath(field):
		return "computed_only"
	default:
		return "unknown"
	}
}

func isLowSignalDriftClass(class string) bool {
	return class == "computed_only" || class == "provider_cache" || class == "timestamp"
}
