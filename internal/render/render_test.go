package render

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/piblokto/tfplanctx/internal/plan"
)

func TestLineRenderer(t *testing.T) {
	p := samplePlan()
	got := RenderLine(p, Options{Limits: DefaultLimits()})
	want := strings.Join([]string{
		"TFP1 C=1 U=1 R=1 D=1 OUT=1 RISK=1",
		"D|aws_db_instance.old|self|exists|null|risk=data_loss",
		"R|aws_instance.app|ami|\"ami-old\"|\"ami-new\"|replace_path",
		"C|aws_s3_bucket.logs|bucket|null|\"prod-logs-example\"|",
		"U|aws_security_group.web|ingress[0].cidr_blocks|[\"10.0.0.0/8\"]|[\"0.0.0.0/0\"]|risk=public_ingress",
		"O|output.endpoint|value|\"old\"|\"new\"|",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("line render mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestJSONLRenderer(t *testing.T) {
	p := samplePlan()
	got, err := RenderJSONL(p, Options{Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 5 {
		t.Fatalf("line count = %d, want 5", len(lines))
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(lines[3]), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["a"] != "U" || decoded["addr"] != "aws_security_group.web" || decoded["p"] != "ingress[0].cidr_blocks" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestSummaryMode(t *testing.T) {
	p := samplePlan()
	got := RenderLine(p, Options{Summary: true, Limits: DefaultLimits()})
	if !strings.Contains(got, "R|aws_instance.app|changes=1|replace_paths=ami") {
		t.Fatalf("summary output missing replace line: %s", got)
	}
	if !strings.Contains(got, "D|aws_db_instance.old|changes=1|risk=data_loss") {
		t.Fatalf("summary output missing delete risk line: %s", got)
	}
}

func TestRiskOnlyMode(t *testing.T) {
	p := samplePlan()
	got := RenderLine(p, Options{RiskOnly: true, Limits: DefaultLimits()})
	if strings.Contains(got, "aws_instance.app") || strings.Contains(got, "aws_s3_bucket.logs") {
		t.Fatalf("risk-only output included safe resources: %s", got)
	}
	if !strings.Contains(got, "aws_db_instance.old") || !strings.Contains(got, "aws_security_group.web") {
		t.Fatalf("risk-only output missing risky resources: %s", got)
	}
}

func samplePlan() *plan.Plan {
	return &plan.Plan{
		Summary: plan.PlanSummary{Creates: 1, Updates: 1, Replaces: 1, Deletes: 1, OutputChanges: 1, RiskResources: 1},
		Resources: []plan.ResourceChange{
			{
				Action:  plan.ActionDelete,
				Address: "aws_db_instance.old",
				Risks:   []plan.Risk{{Name: "data_loss"}},
				Attributes: []plan.AttributeChange{{
					Path:   "self",
					Before: plan.Value{Kind: plan.ValueExists},
					After:  plan.Value{Kind: plan.ValueNull},
					Flags:  []string{"risk=data_loss"},
				}},
			},
			{
				Action:       plan.ActionReplace,
				Address:      "aws_instance.app",
				ReplacePaths: []string{"ami"},
				Attributes: []plan.AttributeChange{{
					Path:   "ami",
					Before: plan.Value{Kind: plan.ValueRaw, Raw: "ami-old"},
					After:  plan.Value{Kind: plan.ValueRaw, Raw: "ami-new"},
					Flags:  []string{"replace_path"},
				}},
			},
			{
				Action:  plan.ActionCreate,
				Address: "aws_s3_bucket.logs",
				Attributes: []plan.AttributeChange{{
					Path:   "bucket",
					Before: plan.Value{Kind: plan.ValueNull},
					After:  plan.Value{Kind: plan.ValueRaw, Raw: "prod-logs-example"},
				}},
			},
			{
				Action:  plan.ActionUpdate,
				Address: "aws_security_group.web",
				Risks:   []plan.Risk{{Name: "public_ingress"}},
				Attributes: []plan.AttributeChange{{
					Path:   "ingress[0].cidr_blocks",
					Before: plan.Value{Kind: plan.ValueRaw, Raw: []any{"10.0.0.0/8"}},
					After:  plan.Value{Kind: plan.ValueRaw, Raw: []any{"0.0.0.0/0"}},
					Flags:  []string{"risk=public_ingress"},
				}},
			},
		},
		Outputs: []plan.OutputChange{{
			Address: "output.endpoint",
			Attributes: []plan.AttributeChange{{
				Path:   "value",
				Before: plan.Value{Kind: plan.ValueRaw, Raw: "old"},
				After:  plan.Value{Kind: plan.ValueRaw, Raw: "new"},
			}},
		}},
	}
}
