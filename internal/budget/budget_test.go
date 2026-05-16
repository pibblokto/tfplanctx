package budget

import (
	"strings"
	"testing"

	"github.com/piblokto/tfplanctx/internal/plan"
	"github.com/piblokto/tfplanctx/internal/render"
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
