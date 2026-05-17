package budget

import (
	"strings"
	"testing"

	"github.com/pibblokto/tfplanctx/internal/plan"
	"github.com/pibblokto/tfplanctx/internal/render"
)

func TestBudgetFallsBackToSummaryAndOmittedHeader(t *testing.T) {
	p := &plan.Plan{
		Summary: plan.PlanSummary{Updates: 1},
		Resources: []plan.ResourceChange{{
			Action:  plan.ActionUpdate,
			Address: "example.big",
			Attributes: []plan.AttributeChange{
				{Path: "a", Before: plan.Value{Kind: plan.ValueRaw, Raw: strings.Repeat("a", 200)}, After: plan.Value{Kind: plan.ValueRaw, Raw: strings.Repeat("b", 200)}},
				{Path: "b", Before: plan.Value{Kind: plan.ValueRaw, Raw: strings.Repeat("c", 200)}, After: plan.Value{Kind: plan.ValueRaw, Raw: strings.Repeat("d", 200)}},
			},
		}},
	}
	output, opts, err := Fit(p, "line", render.Options{Limits: render.DefaultLimits()}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Summary {
		t.Fatalf("expected summary fallback, got opts=%#v", opts)
	}
	if !strings.Contains(output, "OMITTED=1") {
		t.Fatalf("expected omitted header, got %s", output)
	}
	if !strings.Contains(output, "U|example.big|changes=2|") {
		t.Fatalf("expected summary resource line, got %s", output)
	}
}

func TestCompactBudgetKeepsEveryResourceAndMarksOmittedDetails(t *testing.T) {
	p := &plan.Plan{
		Summary: plan.PlanSummary{Creates: 1, Updates: 1},
		Resources: []plan.ResourceChange{
			{
				Action:  plan.ActionCreate,
				Address: "example.create",
				Type:    "example",
				Attributes: []plan.AttributeChange{{
					Path:  "name",
					After: plan.Value{Kind: plan.ValueRaw, Raw: strings.Repeat("a", 200)},
				}},
			},
			{
				Action:       plan.ActionUpdate,
				Address:      "example.update",
				Type:         "example",
				UnknownPaths: []string{"id"},
				Attributes: []plan.AttributeChange{{
					Path:         "id",
					Before:       plan.Value{Kind: plan.ValueRaw, Raw: "old"},
					After:        plan.Value{Kind: plan.ValueUnknown},
					AfterUnknown: true,
				}},
			},
		},
	}
	output, _, err := Fit(p, "compact", render.Options{Limits: render.DefaultLimits()}, 80)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"example.create", "example.update", "OMITTED=", "omit=1", "unk=id"} {
		if !strings.Contains(output, want) {
			t.Fatalf("compact budget output missing %q: %s", want, output)
		}
	}
}

func TestCompactBudgetNeverDropsOutputs(t *testing.T) {
	p := &plan.Plan{
		Summary: plan.PlanSummary{Creates: 1, OutputChanges: 1},
		Resources: []plan.ResourceChange{{
			Action:  plan.ActionCreate,
			Address: "example.big",
			Type:    "example",
			Attributes: []plan.AttributeChange{{
				Path:  "payload",
				After: plan.Value{Kind: plan.ValueRaw, Raw: strings.Repeat("x", 400)},
			}},
		}},
		Outputs: []plan.OutputChange{{
			Name:       "artifact_registry_repository",
			Address:    "output.artifact_registry_repository",
			Action:     plan.ActionUpdate,
			RawActions: []string{"update"},
			Attributes: []plan.AttributeChange{{
				Path:   "value",
				Before: plan.Value{Kind: plan.ValueRaw, Raw: "old"},
				After:  plan.Value{Kind: plan.ValueRaw, Raw: "new"},
			}},
		}},
	}
	output, _, err := Fit(p, "compact", render.Options{Limits: render.DefaultLimits()}, 40)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `O|artifact_registry_repository|"old"->"new"|`) {
		t.Fatalf("compact budget output dropped changed output: %s", output)
	}
}
