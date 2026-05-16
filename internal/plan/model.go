package plan

import "sort"

// Action is the normalized action emitted by tfplanctx.
type Action string

const (
	ActionCreate  Action = "C"
	ActionUpdate  Action = "U"
	ActionDelete  Action = "D"
	ActionReplace Action = "R"
	ActionRead    Action = "Q"
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
	Path         string
	Before       Value
	After        Value
	AfterUnknown bool
	Sensitive    bool
	ReplacePath  bool
	Flags        []string
}

// ResourceChange is one changed Terraform resource with Terraform metadata preserved.
type ResourceChange struct {
	Action          Action
	RawActions      []string
	Address         string
	PreviousAddress string
	Mode            string
	Type            string
	Name            string
	Index           any
	ProviderName    string
	ActionReason    string
	Deposed         string
	Importing       bool
	GeneratedConfig bool
	Attributes      []AttributeChange
	UnknownPaths    []string
	SensitivePaths  []string
	ReplacePaths    []string
	Risks           []Risk
}

// OutputChange is one changed Terraform output.
type OutputChange struct {
	Name           string
	Address        string
	Action         Action
	RawActions     []string
	Attributes     []AttributeChange
	UnknownPaths   []string
	SensitivePaths []string
}

// DriftSummary stores compact drift overview information.
type DriftSummary struct {
	Total         int
	RiskResources int
	Types         []string
}

// PlanMetadata stores plan-wide metadata not tied to a single rendered value.
type PlanMetadata struct {
	CheckCount             int
	FailedCheckCount       int
	ImportCount            int
	GeneratedConfigCount   int
	RelevantFailureCount   int
	RelevantAttributeCount int
}

// PlanSummary stores normalized plan counts.
type PlanSummary struct {
	Creates       int
	Updates       int
	Replaces      int
	Deletes       int
	Reads         int
	OutputChanges int
	RiskResources int
}

// Plan is the renderer-facing normalized model.
type Plan struct {
	Summary       PlanSummary
	Resources     []ResourceChange
	Outputs       []OutputChange
	NoOpResources []ResourceChange
	Drift         []ResourceChange
	DriftSummary  DriftSummary
	Metadata      PlanMetadata
}

// Filter returns a shallow filtered copy with recalculated summaries.
func (p *Plan) Filter(address, resourceType string) *Plan {
	filtered := &Plan{Metadata: p.Metadata}
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
	for _, drift := range p.Drift {
		if address != "" && drift.Address != address {
			continue
		}
		if resourceType != "" && drift.Type != resourceType {
			continue
		}
		filtered.Drift = append(filtered.Drift, drift)
	}
	filtered.Summary = summarize(filtered)
	filtered.DriftSummary = summarizeDrift(filtered.Drift)
	return filtered
}

// HasChanges reports whether the original normalized plan contains material changes.
func (p *Plan) HasChanges() bool {
	return p.Summary.Creates > 0 || p.Summary.Updates > 0 || p.Summary.Replaces > 0 || p.Summary.Deletes > 0 || p.Summary.Reads > 0 || p.Summary.OutputChanges > 0
}

// HasRisks reports whether any changed resource carries a risk annotation.
func (p *Plan) HasRisks() bool {
	return p.Summary.RiskResources > 0
}

// Sort applies the stable global ordering contract.
func (p *Plan) Sort() {
	sortResources(p.Resources)
	sortResources(p.NoOpResources)
	sortResources(p.Drift)
	for i := range p.Outputs {
		sort.Slice(p.Outputs[i].Attributes, func(a, b int) bool {
			return p.Outputs[i].Attributes[a].Path < p.Outputs[i].Attributes[b].Path
		})
		sort.Strings(p.Outputs[i].UnknownPaths)
		sort.Strings(p.Outputs[i].SensitivePaths)
	}
	sort.Slice(p.Outputs, func(i, j int) bool {
		return p.Outputs[i].Address < p.Outputs[j].Address
	})
}

func sortResources(resources []ResourceChange) {
	sort.Slice(resources, func(i, j int) bool {
		left, right := resources[i], resources[j]
		if actionRank(left.Action) != actionRank(right.Action) {
			return actionRank(left.Action) < actionRank(right.Action)
		}
		return left.Address < right.Address
	})
	for i := range resources {
		sort.Slice(resources[i].Attributes, func(a, b int) bool {
			return resources[i].Attributes[a].Path < resources[i].Attributes[b].Path
		})
		sort.Strings(resources[i].UnknownPaths)
		sort.Strings(resources[i].SensitivePaths)
		sort.Strings(resources[i].ReplacePaths)
	}
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
	case ActionRead:
		return 4
	case ActionOutput:
		return 5
	default:
		return 6
	}
}
