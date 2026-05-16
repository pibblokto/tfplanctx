package plan

import "sort"

// Action is the normalized action emitted by tfplanctx.
type Action string

const (
	ActionCreate  Action = "C"
	ActionUpdate  Action = "U"
	ActionDelete  Action = "D"
	ActionReplace Action = "R"
	ActionOutput  Action = "O"
	ActionNoOp    Action = "N"
)

// ValueKind describes how a value should be rendered safely.
type ValueKind string

const (
	ValueRaw       ValueKind = "raw"
	ValueNull      ValueKind = "null"
	ValueUnknown   ValueKind = "unknown"
	ValueSensitive ValueKind = "sensitive"
	ValueExists    ValueKind = "exists"
)

// Value is a normalized attribute value.
type Value struct {
	Kind ValueKind
	Raw  any
}

// Risk is one deterministic rule annotation.
type Risk struct {
	Name string
}

// AttributeChange is one changed leaf attribute.
type AttributeChange struct {
	Path   string
	Before Value
	After  Value
	Flags  []string
}

// ResourceChange is one changed Terraform resource.
type ResourceChange struct {
	Action       Action
	Address      string
	Type         string
	Attributes   []AttributeChange
	ReplacePaths []string
	Risks        []Risk
}

// OutputChange is one changed Terraform output.
type OutputChange struct {
	Address    string
	Attributes []AttributeChange
}

// PlanSummary stores normalized plan counts.
type PlanSummary struct {
	Creates       int
	Updates       int
	Replaces      int
	Deletes       int
	OutputChanges int
	RiskResources int
}

// Plan is the renderer-facing normalized model.
type Plan struct {
	Summary       PlanSummary
	Resources     []ResourceChange
	Outputs       []OutputChange
	NoOpResources []ResourceChange
}

// Filter returns a shallow filtered copy while preserving the original summary header.
func (p *Plan) Filter(address, resourceType string) *Plan {
	filtered := &Plan{}
	for _, resource := range p.Resources {
		if address != "" && resource.Address != address {
			continue
		}
		if resourceType != "" && resource.Type != resourceType {
			continue
		}
		filtered.Resources = append(filtered.Resources, resource)
	}
	for _, output := range p.Outputs {
		if address != "" && output.Address != address {
			continue
		}
		if resourceType != "" {
			continue
		}
		filtered.Outputs = append(filtered.Outputs, output)
	}
	for _, resource := range p.NoOpResources {
		if address != "" && resource.Address != address {
			continue
		}
		if resourceType != "" && resource.Type != resourceType {
			continue
		}
		filtered.NoOpResources = append(filtered.NoOpResources, resource)
	}
	filtered.Summary = summarize(filtered)
	return filtered
}

// HasChanges reports whether the original normalized plan contains material changes.
func (p *Plan) HasChanges() bool {
	return p.Summary.Creates > 0 || p.Summary.Updates > 0 || p.Summary.Replaces > 0 || p.Summary.Deletes > 0 || p.Summary.OutputChanges > 0
}

// HasRisks reports whether any changed resource carries a risk annotation.
func (p *Plan) HasRisks() bool {
	return p.Summary.RiskResources > 0
}

// Sort applies the stable global ordering contract.
func (p *Plan) Sort() {
	sort.Slice(p.Resources, func(i, j int) bool {
		left, right := p.Resources[i], p.Resources[j]
		if actionRank(left.Action) != actionRank(right.Action) {
			return actionRank(left.Action) < actionRank(right.Action)
		}
		return left.Address < right.Address
	})
	for i := range p.Resources {
		sort.Slice(p.Resources[i].Attributes, func(a, b int) bool {
			return p.Resources[i].Attributes[a].Path < p.Resources[i].Attributes[b].Path
		})
	}
	for i := range p.Outputs {
		sort.Slice(p.Outputs[i].Attributes, func(a, b int) bool {
			return p.Outputs[i].Attributes[a].Path < p.Outputs[i].Attributes[b].Path
		})
	}
	sort.Slice(p.Outputs, func(i, j int) bool {
		return p.Outputs[i].Address < p.Outputs[j].Address
	})
	sort.Slice(p.NoOpResources, func(i, j int) bool {
		return p.NoOpResources[i].Address < p.NoOpResources[j].Address
	})
}

func actionRank(action Action) int {
	switch action {
	case ActionDelete:
		return 0
	case ActionReplace:
		return 1
	case ActionCreate:
		return 2
	case ActionUpdate:
		return 3
	case ActionOutput:
		return 4
	default:
		return 5
	}
}
