package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/piblokto/tfplanctx/internal/diff"
	"github.com/piblokto/tfplanctx/internal/redact"
	"github.com/piblokto/tfplanctx/internal/risk"
)

// ParseOptions configures normalization behavior.
type ParseOptions struct {
	IncludeRead bool
	Redact      redact.Config
}

type rawPlan struct {
	ResourceChanges []rawResourceChange        `json:"resource_changes"`
	OutputChanges   map[string]rawOutputChange `json:"output_changes"`
}

type rawResourceChange struct {
	Address string    `json:"address"`
	Mode    string    `json:"mode"`
	Type    string    `json:"type"`
	Name    string    `json:"name"`
	Change  rawChange `json:"change"`
}

type rawOutputChange struct {
	Change rawChange `json:"change"`
}

type rawChange struct {
	Actions         []string `json:"actions"`
	Before          any      `json:"before"`
	After           any      `json:"after"`
	AfterUnknown    any      `json:"after_unknown"`
	BeforeSensitive any      `json:"before_sensitive"`
	AfterSensitive  any      `json:"after_sensitive"`
	ReplacePaths    [][]any  `json:"replace_paths"`
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
		action, include, noop := normalizeAction(rawResource.Change.Actions, opts.IncludeRead)
		if noop {
			normalized.NoOpResources = append(normalized.NoOpResources, ResourceChange{
				Action:  ActionNoOp,
				Address: rawResource.Address,
				Type:    rawResource.Type,
			})
			continue
		}
		if !include {
			continue
		}

		rawChanges := diff.Changes(rawResource.Change.Before, rawResource.Change.After, rawResource.Change.AfterUnknown)
		replacePaths := normalizeReplacePaths(rawResource.Change.ReplacePaths)
		resourceRisks := normalizeRisks(risk.Detect(risk.Resource{
			Type:    rawResource.Type,
			Action:  string(action),
			Before:  rawResource.Change.Before,
			After:   rawResource.Change.After,
			Changes: rawChanges,
		}))

		resource := ResourceChange{
			Action:       action,
			Address:      rawResource.Address,
			Type:         rawResource.Type,
			ReplacePaths: replacePaths,
			Risks:        resourceRisks,
		}
		for _, rawChange := range rawChanges {
			path := rawChange.Path.String()
			if path == "" {
				path = "self"
			}

			attribute := AttributeChange{
				Path: path,
			}
			if redact.ShouldRedact(rawChange.Path, rawResource.Change.BeforeSensitive, rawResource.Change.AfterSensitive, opts.Redact) {
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
			if matchesReplacePath(rawChange.Path, rawResource.Change.ReplacePaths) {
				attribute.Flags = append(attribute.Flags, "replace_path")
			}
			attribute.Flags = append(attribute.Flags, riskFlags(resourceRisks)...)
			resource.Attributes = append(resource.Attributes, attribute)
		}

		if action == ActionDelete && len(resource.Attributes) == 0 {
			resource.Attributes = append(resource.Attributes, AttributeChange{
				Path:   "self",
				Before: Value{Kind: ValueExists},
				After:  Value{Kind: ValueNull},
				Flags:  riskFlags(resourceRisks),
			})
		}

		normalized.Resources = append(normalized.Resources, resource)
	}

	outputNames := make([]string, 0, len(raw.OutputChanges))
	for name := range raw.OutputChanges {
		outputNames = append(outputNames, name)
	}
	sort.Strings(outputNames)
	for _, name := range outputNames {
		rawOutput := raw.OutputChanges[name]
		_, include, noop := normalizeAction(rawOutput.Change.Actions, true)
		if noop || !include {
			continue
		}
		changes := diff.Changes(rawOutput.Change.Before, rawOutput.Change.After, rawOutput.Change.AfterUnknown)
		if len(changes) == 0 {
			continue
		}
		output := OutputChange{Address: "output." + name}
		for _, rawChange := range changes {
			path := rawChange.Path.String()
			if path == "" {
				path = "value"
			}
			attribute := AttributeChange{Path: path}
			if redact.ShouldRedact(rawChange.Path, rawOutput.Change.BeforeSensitive, rawOutput.Change.AfterSensitive, opts.Redact) {
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
			output.Attributes = append(output.Attributes, attribute)
		}
		normalized.Outputs = append(normalized.Outputs, output)
	}

	normalized.Sort()
	normalized.Summary = summarize(normalized)
	return normalized, nil
}

func normalizeAction(actions []string, includeRead bool) (Action, bool, bool) {
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
			return ActionUpdate, includeRead, false
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
	sort.Strings(values)
	return values
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
		}
		if len(resource.Risks) > 0 {
			summary.RiskResources++
		}
	}
	summary.OutputChanges = len(p.Outputs)
	return summary
}

// RiskNames returns normalized risk names for a resource.
func (r ResourceChange) RiskNames() []string {
	values := make([]string, 0, len(r.Risks))
	for _, item := range r.Risks {
		values = append(values, item.Name)
	}
	return values
}

// SummaryFlags returns the stable flag list used by summary renderers.
func (r ResourceChange) SummaryFlags() []string {
	var flags []string
	if len(r.ReplacePaths) > 0 {
		flags = append(flags, "replace_paths="+strings.Join(r.ReplacePaths, ","))
	}
	flags = append(flags, riskFlags(r.Risks)...)
	return flags
}
