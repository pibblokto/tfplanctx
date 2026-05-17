package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pibblokto/tfplanctx/internal/diff"
	"github.com/pibblokto/tfplanctx/internal/redact"
	"github.com/pibblokto/tfplanctx/internal/risk"
)

// ParseOptions configures normalization behavior.
type ParseOptions struct {
	// IncludeRead is retained for CLI compatibility. Reads are now normalized by default
	// so every non-no-op resource_change survives the compact pipeline.
	IncludeRead bool
	Redact      redact.Config
}

type rawPlan struct {
	ResourceChanges    []rawResourceChange        `json:"resource_changes"`
	ResourceDrift      []rawResourceChange        `json:"resource_drift"`
	OutputChanges      map[string]rawOutputChange `json:"output_changes"`
	Checks             []rawCheck                 `json:"checks"`
	RelevantAttributes []json.RawMessage          `json:"relevant_attributes"`
}

type rawCheck struct {
	Status string `json:"status"`
}

type rawResourceChange struct {
	Address         string          `json:"address"`
	PreviousAddress string          `json:"previous_address"`
	Mode            string          `json:"mode"`
	Type            string          `json:"type"`
	Name            string          `json:"name"`
	Index           any             `json:"index"`
	ProviderName    string          `json:"provider_name"`
	Deposed         string          `json:"deposed"`
	ActionReason    string          `json:"action_reason"`
	GeneratedConfig json.RawMessage `json:"generated_config"`
	Change          rawChange       `json:"change"`
}

type rawOutputChange struct {
	Change rawChange `json:"change"`
	rawChange
}

type rawChange struct {
	Actions         []string        `json:"actions"`
	Before          any             `json:"before"`
	After           any             `json:"after"`
	AfterUnknown    any             `json:"after_unknown"`
	BeforeSensitive any             `json:"before_sensitive"`
	AfterSensitive  any             `json:"after_sensitive"`
	ReplacePaths    [][]any         `json:"replace_paths"`
	Importing       json.RawMessage `json:"importing"`
	GeneratedConfig json.RawMessage `json:"generated_config"`
}

// Parse converts Terraform JSON plan output into the normalized model.
func Parse(data []byte, opts ParseOptions) (*Plan, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var raw rawPlan
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse terraform JSON plan: %w", err)
	}

	normalized := &Plan{}
	for _, rawResource := range raw.ResourceChanges {
		resource, noop, include := normalizeResource(rawResource, opts)
		if noop {
			resource.Action = ActionNoOp
			normalized.NoOpResources = append(normalized.NoOpResources, resource)
			continue
		}
		if !include {
			continue
		}
		normalized.Resources = append(normalized.Resources, resource)
		if resource.Importing {
			normalized.Metadata.ImportCount++
		}
		if resource.GeneratedConfig {
			normalized.Metadata.GeneratedConfigCount++
		}
	}

	for _, rawDrift := range raw.ResourceDrift {
		drift, noop, include := normalizeResource(rawDrift, opts)
		if noop || !include {
			continue
		}
		normalized.Drift = append(normalized.Drift, drift)
	}

	outputNames := make([]string, 0, len(raw.OutputChanges))
	for name := range raw.OutputChanges {
		outputNames = append(outputNames, name)
	}
	sort.Strings(outputNames)
	for _, name := range outputNames {
		rawOutput := raw.OutputChanges[name]
		change := rawOutput.effectiveChange()
		action, include, noop := normalizeAction(change.Actions)
		if noop || !include {
			continue
		}
		output := OutputChange{
			Name:           name,
			Address:        "output." + name,
			Action:         action,
			RawActions:     append([]string(nil), change.Actions...),
			UnknownPaths:   normalizeOutputPaths(flattenMarkedPaths(change.AfterUnknown)),
			SensitivePaths: normalizeOutputPaths(mergePaths(flattenMarkedPaths(change.BeforeSensitive), flattenMarkedPaths(change.AfterSensitive))),
		}
		attribute := AttributeChange{
			Path:         "value",
			AfterUnknown: hasRootMarker(change.AfterUnknown),
			Sensitive:    len(output.SensitivePaths) > 0 || redact.ShouldRedact(nil, change.BeforeSensitive, change.AfterSensitive, opts.Redact),
		}
		if attribute.Sensitive {
			attribute.Before = Value{Kind: ValueSensitive}
			attribute.After = Value{Kind: ValueSensitive}
		} else {
			attribute.Before = valueFromRaw(change.Before)
			if attribute.AfterUnknown {
				attribute.After = Value{Kind: ValueUnknown}
			} else {
				attribute.After = valueFromRaw(change.After)
			}
		}
		output.Attributes = append(output.Attributes, attribute)
		normalized.Outputs = append(normalized.Outputs, output)
	}

	normalized.Metadata.CheckCount = len(raw.Checks)
	for _, check := range raw.Checks {
		if check.Status == "fail" || check.Status == "error" {
			normalized.Metadata.FailedCheckCount++
		}
	}
	normalized.Metadata.RelevantAttributeCount = len(raw.RelevantAttributes)
	normalized.Sort()
	normalized.Summary = summarize(normalized)
	normalized.DriftSummary = summarizeDrift(normalized.Drift)
	return normalized, nil
}

func normalizeResource(rawResource rawResourceChange, opts ParseOptions) (ResourceChange, bool, bool) {
	action, include, noop := normalizeAction(rawResource.Change.Actions)
	resource := ResourceChange{
		Action:          action,
		RawActions:      append([]string(nil), rawResource.Change.Actions...),
		Address:         rawResource.Address,
		PreviousAddress: rawResource.PreviousAddress,
		Mode:            rawResource.Mode,
		Type:            rawResource.Type,
		Name:            rawResource.Name,
		Index:           rawResource.Index,
		ProviderName:    rawResource.ProviderName,
		ActionReason:    rawResource.ActionReason,
		Deposed:         rawResource.Deposed,
		Importing:       hasJSONValue(rawResource.Change.Importing),
		GeneratedConfig: hasJSONValue(rawResource.GeneratedConfig) || hasJSONValue(rawResource.Change.GeneratedConfig),
		ReplacePaths:    normalizeReplacePaths(rawResource.Change.ReplacePaths),
		UnknownPaths:    flattenMarkedPaths(rawResource.Change.AfterUnknown),
		SensitivePaths:  mergePaths(flattenMarkedPaths(rawResource.Change.BeforeSensitive), flattenMarkedPaths(rawResource.Change.AfterSensitive)),
	}
	if noop || !include {
		return resource, noop, include
	}

	rawChanges := diff.Changes(rawResource.Change.Before, rawResource.Change.After, rawResource.Change.AfterUnknown)
	resource.Risks = normalizeRisks(risk.Detect(risk.Resource{
		Type:    rawResource.Type,
		Action:  string(action),
		Before:  rawResource.Change.Before,
		After:   rawResource.Change.After,
		Changes: rawChanges,
	}))
	for _, rawChange := range rawChanges {
		path := rawChange.Path.String()
		if path == "" {
			path = "self"
		}

		attribute := AttributeChange{
			Path:         path,
			AfterUnknown: rawChange.AfterUnknown,
			ReplacePath:  matchesReplacePath(rawChange.Path, rawResource.Change.ReplacePaths),
		}
		attribute.Sensitive = redact.ShouldRedact(rawChange.Path, rawResource.Change.BeforeSensitive, rawResource.Change.AfterSensitive, opts.Redact)
		if attribute.Sensitive {
			attribute.Before = Value{Kind: ValueSensitive}
			attribute.After = Value{Kind: ValueSensitive}
		} else {
			attribute.Before = valueFromRaw(rawChange.Before)
			if rawChange.AfterUnknown {
				attribute.After = Value{Kind: ValueUnknown}
			} else {
				attribute.After = valueFromRaw(rawChange.After)
			}
		}
		if attribute.ReplacePath {
			attribute.Flags = append(attribute.Flags, "replace_path")
		}
		attribute.Flags = append(attribute.Flags, riskFlags(resource.Risks)...)
		resource.Attributes = append(resource.Attributes, attribute)
	}
	for _, attr := range resource.Attributes {
		if attr.Sensitive && !contains(resource.SensitivePaths, attr.Path) {
			resource.SensitivePaths = append(resource.SensitivePaths, attr.Path)
		}
	}
	resource.SensitivePaths = uniqueSorted(resource.SensitivePaths)
	return resource, false, true
}

func normalizeAction(actions []string) (Action, bool, bool) {
	if len(actions) == 0 {
		return "", false, false
	}
	if len(actions) == 1 {
		switch actions[0] {
		case "create":
			return ActionCreate, true, false
		case "update":
			return ActionUpdate, true, false
		case "delete":
			return ActionDelete, true, false
		case "read":
			return ActionRead, true, false
		case "no-op":
			return ActionNoOp, false, true
		}
	}
	if len(actions) == 2 {
		if (actions[0] == "delete" && actions[1] == "create") || (actions[0] == "create" && actions[1] == "delete") {
			return ActionReplace, true, false
		}
	}
	return "", false, false
}

func (o rawOutputChange) effectiveChange() rawChange {
	if len(o.Change.Actions) > 0 {
		return o.Change
	}
	return o.rawChange
}

func valueFromRaw(raw any) Value {
	if raw == nil {
		return Value{Kind: ValueNull}
	}
	return Value{Kind: ValueRaw, Raw: raw}
}

func normalizeReplacePaths(paths [][]any) []string {
	values := make([]string, 0, len(paths))
	for _, rawPath := range paths {
		path := diff.FromRawPath(rawPath).String()
		if path != "" {
			values = append(values, path)
		}
	}
	return uniqueSorted(values)
}

func normalizeRisks(names []string) []Risk {
	risks := make([]Risk, 0, len(names))
	for _, name := range names {
		risks = append(risks, Risk{Name: name})
	}
	return risks
}

func riskFlags(risks []Risk) []string {
	flags := make([]string, 0, len(risks))
	for _, item := range risks {
		flags = append(flags, "risk="+item.Name)
	}
	return flags
}

func matchesReplacePath(path diff.Path, rawPaths [][]any) bool {
	for _, rawPath := range rawPaths {
		replacePath := diff.FromRawPath(rawPath)
		if path.HasPrefix(replacePath) {
			return true
		}
	}
	return false
}

func flattenMarkedPaths(value any) []string {
	var paths []string
	walkMarkedPaths(nil, value, &paths)
	return uniqueSorted(paths)
}

func walkMarkedPaths(path diff.Path, value any, paths *[]string) {
	switch typed := value.(type) {
	case bool:
		if typed {
			name := path.String()
			if name == "" {
				name = "self"
			}
			*paths = append(*paths, name)
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			walkMarkedPaths(path.WithKey(key), typed[key], paths)
		}
	case []any:
		for index, child := range typed {
			walkMarkedPaths(path.WithIndex(index), child, paths)
		}
	}
}

func mergePaths(groups ...[]string) []string {
	var merged []string
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return uniqueSorted(merged)
}

func normalizeOutputPaths(paths []string) []string {
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "self" {
			normalized = append(normalized, "value")
			continue
		}
		normalized = append(normalized, "value."+path)
	}
	return uniqueSorted(normalized)
}

func hasRootMarker(value any) bool {
	marked, ok := value.(bool)
	return ok && marked
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func hasJSONValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte("{}"))
}

func summarize(p *Plan) PlanSummary {
	var summary PlanSummary
	for _, resource := range p.Resources {
		switch resource.Action {
		case ActionCreate:
			summary.Creates++
		case ActionUpdate:
			summary.Updates++
		case ActionReplace:
			summary.Replaces++
		case ActionDelete:
			summary.Deletes++
		case ActionRead:
			summary.Reads++
		}
		if len(resource.Risks) > 0 {
			summary.RiskResources++
		}
	}
	summary.OutputChanges = len(p.Outputs)
	return summary
}

func summarizeDrift(resources []ResourceChange) DriftSummary {
	types := make(map[string]struct{})
	var summary DriftSummary
	for _, resource := range resources {
		summary.Total++
		if resource.Type != "" {
			types[resource.Type] = struct{}{}
		}
		if len(resource.Risks) > 0 {
			summary.RiskResources++
		}
	}
	for resourceType := range types {
		summary.Types = append(summary.Types, resourceType)
	}
	sort.Strings(summary.Types)
	return summary
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// RiskNames returns normalized risk names for a resource.
func (r ResourceChange) RiskNames() []string {
	values := make([]string, 0, len(r.Risks))
	for _, item := range r.Risks {
		values = append(values, item.Name)
	}
	return values
}

// SummaryFlags returns the stable flag list used by legacy summary renderers.
func (r ResourceChange) SummaryFlags() []string {
	var flags []string
	if len(r.ReplacePaths) > 0 {
		flags = append(flags, "replace_paths="+strings.Join(r.ReplacePaths, ","))
	}
	if r.ActionReason != "" {
		flags = append(flags, "reason="+r.ActionReason)
	}
	if len(r.UnknownPaths) > 0 {
		flags = append(flags, "unknown="+strings.Join(r.UnknownPaths, ","))
	}
	if len(r.SensitivePaths) > 0 {
		flags = append(flags, "sensitive="+strings.Join(r.SensitivePaths, ","))
	}
	flags = append(flags, riskFlags(r.Risks)...)
	return flags
}
