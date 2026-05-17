package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pibblokto/tfplanctx/internal/plan"
	"github.com/pibblokto/tfplanctx/internal/verify"
)

func TestCompactRendererUsesResourceScopedTFP2(t *testing.T) {
	parsed := mustParseCompactFixture(t, "plan_main.json")
	got := RenderCompact(parsed, Options{Limits: DefaultLimits()})
	if !strings.HasPrefix(got, "TFP2 ") {
		t.Fatalf("missing TFP2 header: %s", got)
	}
	if !strings.Contains(got, `R|aws_instance.app|ami="ami-old"->"ami-new"|acts=delete,create;repl=ami`) {
		t.Fatalf("replace record missing or malformed: %s", got)
	}
	if strings.Count(got, "aws_security_group.web") != 1 {
		t.Fatalf("resource address repeated unexpectedly: %s", got)
	}
	if err := verify.Compact(parsed, got); err != nil {
		t.Fatalf("compact verification failed: %v\n%s", err, got)
	}
}

func TestCompactRendererPreservesMetadataForNoMaterialAttributes(t *testing.T) {
	parsed := mustParseCompactFixture(t, "plan_tfp2_coverage.json")
	got := RenderCompact(parsed, Options{Limits: DefaultLimits()})
	for _, want := range []string{
		`C|module.foo.terraform_data.validation||type=terraform_data;unk=id;attrs=none;no_material_attrs=true`,
		`why=delete_because_no_resource_config`,
		`repl=identifier`,
		`sens=password`,
		`DRIFT|total=1;risk=1;summ=0;detail=1`,
		`META|checks=2;check_fail=1;imports=1;generated_config=1;relevant_attrs=1`,
		`labels["a.b[0]%7Cx"]="old"->"new"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact output missing %q\n%s", want, got)
		}
	}
	if err := verify.Compact(parsed, got); err != nil {
		t.Fatalf("compact verification failed: %v\n%s", err, got)
	}
}

func TestCompactRendererIsSmallerThanTFP1OnRepetitivePlans(t *testing.T) {
	parsed := mustParseCompactFixture(t, "plan_repetitive.json")
	compact := RenderCompact(parsed, Options{Limits: DefaultLimits()})
	detail := RenderCompact(parsed, Options{Detail: true, Limits: DefaultLimits()})
	legacy := RenderLine(parsed, Options{Limits: DefaultLimits()})
	if len(compact) >= len(legacy) {
		t.Fatalf("compact output len=%d, legacy len=%d\ncompact:\n%s\nlegacy:\n%s", len(compact), len(legacy), compact, legacy)
	}
	if len(compact) >= len(detail) {
		t.Fatalf("review output len=%d, detail len=%d\nreview:\n%s\ndetail:\n%s", len(compact), len(detail), compact, detail)
	}
	for _, want := range []string{
		`GL|G1|C|google_project_iam_member|`,
		`G|G2|D|google_project_iam_member|`,
		`TPL|P1|module.a.google_project_iam_member.`,
		`REASON_CODES|no_config=delete_because_no_resource_config`,
		`OMIT|default_empty=11`,
		`COMPRESS|grouped_common=22;groups=2;templates=2`,
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("compact review output missing %q: %s", want, compact)
		}
	}
	if strings.Contains(compact, "OMIT|grouped_common=") {
		t.Fatalf("non-lossy grouped_common was counted as omitted:\n%s", compact)
	}
}

func TestCompactDetailKeepsExpandedResourceRecords(t *testing.T) {
	parsed := mustParseCompactFixture(t, "plan_main.json")
	got := RenderCompact(parsed, Options{Detail: true, Limits: DefaultLimits()})
	for _, want := range []string{
		`R|aws_instance.app|ami="ami-old"->"ami-new"|actions=delete,create;replace_path=ami`,
		`U|aws_instance.unknown|id="i-123"->unknown|unknown=id`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact detail output missing %q: %s", want, got)
		}
	}
}

func TestCompactRendererPreservesOutputActionsAndReviewSummaries(t *testing.T) {
	parsed := mustParseCompactFixture(t, "plan_output_changes.json")
	review := RenderCompact(parsed, Options{Limits: DefaultLimits()})
	for _, want := range []string{
		`TFP2 C=0 U=0 R=0 D=0 Q=0 OUT=7 RISK=0 DRIFT=0 OMITTED=1`,
		`OMIT|summarized=1`,
		`O|created|+"new-value"|`,
		`O|deleted|-"old-value"|`,
		`O|updated|"old-value"->"new-value"|`,
		`O|unknown_after|"old-value"->unknown|unk=value`,
		`O|sensitive_output|sensitive->sensitive|sens=value`,
		`O|complex_output|{"enabled":false,"ports":[80]}->{"enabled":true,"ports":[80,443]}|`,
		`O|long_output|long_string(len%3D184,sha256%3D`,
		`summary=true;detail_required=true`,
	} {
		if !strings.Contains(review, want) {
			t.Fatalf("review output missing %q:\n%s", want, review)
		}
	}
	if err := verify.Compact(parsed, review); err != nil {
		t.Fatalf("review output did not verify: %v\n%s", err, review)
	}

	detail := RenderCompact(parsed, Options{Detail: true, Limits: DefaultLimits()})
	if strings.Contains(detail, `long_string(`) {
		t.Fatalf("detail output summarized a long output:\n%s", detail)
	}
	if !strings.Contains(detail, strings.Repeat("a", 184)) || !strings.Contains(detail, strings.Repeat("b", 176)) {
		t.Fatalf("detail output lost exact long values:\n%s", detail)
	}
	if err := verify.Compact(parsed, detail); err != nil {
		t.Fatalf("detail output did not verify: %v\n%s", err, detail)
	}
}

func TestCompactDetailKeepsExactLongResourceValues(t *testing.T) {
	longBefore := strings.Repeat("a", 220)
	longAfter := strings.Repeat("b", 220)
	p := &plan.Plan{
		Summary: plan.PlanSummary{Updates: 1},
		Resources: []plan.ResourceChange{{
			Action:  plan.ActionUpdate,
			Address: "terraform_data.group_settings",
			Type:    "terraform_data",
			Attributes: []plan.AttributeChange{{
				Path:   "settings",
				Before: plan.Value{Kind: plan.ValueRaw, Raw: longBefore},
				After:  plan.Value{Kind: plan.ValueRaw, Raw: longAfter},
			}},
		}},
	}
	review := RenderCompact(p, Options{Limits: DefaultLimits()})
	for _, want := range []string{`long_string(`, `summ=settings`, `detail_required=true`, `OMIT|summarized=1`} {
		if !strings.Contains(review, want) {
			t.Fatalf("review output missing %q:\n%s", want, review)
		}
	}
	detail := RenderCompact(p, Options{Detail: true, Limits: DefaultLimits()})
	if strings.Contains(detail, `long_string(`) || !strings.Contains(detail, longBefore) || !strings.Contains(detail, longAfter) {
		t.Fatalf("detail output is not exact:\n%s", detail)
	}
}

func TestCompactReviewSummarizesLowSignalDrift(t *testing.T) {
	p := &plan.Plan{
		Summary: plan.PlanSummary{},
		Drift: []plan.ResourceChange{{
			Action:  plan.ActionUpdate,
			Address: "example.cache",
			Type:    "example",
			Attributes: []plan.AttributeChange{{
				Path:   "etag",
				Before: plan.Value{Kind: plan.ValueRaw, Raw: "old"},
				After:  plan.Value{Kind: plan.ValueRaw, Raw: "new"},
			}},
		}},
		DriftSummary: plan.DriftSummary{Total: 1, Types: []string{"example"}},
	}
	got := RenderCompact(p, Options{Limits: DefaultLimits()})
	if !strings.Contains(got, `DRIFT_GROUP|type=example;count=1;fields=etag;class=provider_cache`) {
		t.Fatalf("low-signal drift was not grouped: %s", got)
	}
	if strings.Contains(got, `DRIFT_DETAIL|`) {
		t.Fatalf("low-signal drift unexpectedly expanded: %s", got)
	}
}

func TestCompactReviewEscapesValues(t *testing.T) {
	p := &plan.Plan{
		Summary: plan.PlanSummary{Creates: 3},
		Resources: []plan.ResourceChange{
			groupedValueResource(`module.long_path.example_resource.item_0`, `a|b`),
			groupedValueResource(`module.long_path.example_resource.item_1`, `semi;eq=`),
			groupedValueResource(`module.long_path.example_resource.item_2`, "line\nnext"),
		},
	}
	got := RenderCompact(p, Options{Limits: DefaultLimits()})
	for _, want := range []string{"VAL|V1|", "%7C", "%3B", "%3D"} {
		if !strings.Contains(got, want) {
			t.Fatalf("grouped output missing escaped token %q: %s", want, got)
		}
	}
	if err := verify.Compact(p, got); err != nil {
		t.Fatalf("grouped output did not parse: %v\n%s", err, got)
	}
}

func TestCompactReviewUsesIAMLensDictionariesAndMigrationSummary(t *testing.T) {
	parsed := mustParseCompactFixture(t, "plan_lens_dictionary.json")
	got := RenderCompact(parsed, Options{Limits: DefaultLimits()})
	for _, want := range []string{
		`VAL|V1|"group:backend-developers@example.com"`,
		`L|IAM|I1|C|google_project_iam_member|`,
		`roles=`,
		`refs=`,
		`MIGRATION?|type=google_project_iam_member;C=4;D=3;same_scope=project:"deepsearch-dev";common_roles=`,
		`R|google_artifact_registry_repository.deepsearch_dev_repository|repository_id="deepsearch-backend-repository"->"deepsearch-backend-repository2"|acts=delete,create;repl=repository_id`,
		`U|google_dns_record_set.dns_record|ttl=300->400|`,
		`O|artifact_registry_repository|"deepsearch-backend-repository"->"deepsearch-backend-repository2-adasd"|`,
		`DRIFT|total=1;risk=0;summ=1;detail=0`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact review output missing %q:\n%s", want, got)
		}
	}
	if err := verify.Compact(parsed, got); err != nil {
		t.Fatalf("compact verification failed: %v\n%s", err, got)
	}
	stats := CompactReviewStats(parsed, Options{Limits: DefaultLimits()})
	if stats.LensResources != 7 {
		t.Fatalf("lens resources = %d, want 7", stats.LensResources)
	}
	if stats.DictionaryCount == 0 {
		t.Fatal("expected value dictionary entries")
	}
}

func TestCompactReviewUsesGenericListGroupWhenCheaper(t *testing.T) {
	p := &plan.Plan{
		Summary: plan.PlanSummary{Creates: 8},
	}
	for i := 0; i < 8; i++ {
		p.Resources = append(p.Resources, listGroupResource(fmt.Sprintf("module.long_example.example_role.item_%d", i), fmt.Sprintf("role-%d", i)))
	}
	got := RenderCompact(p, Options{Limits: DefaultLimits()})
	if !strings.Contains(got, "GL|") {
		t.Fatalf("expected generic scalar list group:\n%s", got)
	}
	if err := verify.Compact(p, got); err != nil {
		t.Fatalf("list group did not verify: %v\n%s", err, got)
	}
}

func groupedValueResource(address, value string) plan.ResourceChange {
	return plan.ResourceChange{
		Action:  plan.ActionCreate,
		Address: address,
		Type:    "example_resource",
		Attributes: []plan.AttributeChange{
			{Path: "project", After: plan.Value{Kind: plan.ValueRaw, Raw: strings.Repeat("project-", 8)}},
			{Path: "value", After: plan.Value{Kind: plan.ValueRaw, Raw: value}},
		},
	}
}

func listGroupResource(address, value string) plan.ResourceChange {
	return plan.ResourceChange{
		Action:  plan.ActionCreate,
		Address: address,
		Type:    "example_role",
		Attributes: []plan.AttributeChange{
			{Path: "namespace", After: plan.Value{Kind: plan.ValueRaw, Raw: "shared"}},
			{Path: "role", After: plan.Value{Kind: plan.ValueRaw, Raw: value}},
		},
	}
}

func TestCompactRendererKeepsMeaningfulEmptyTransitions(t *testing.T) {
	p := &plan.Plan{
		Summary: plan.PlanSummary{Updates: 1},
		Resources: []plan.ResourceChange{{
			Action:  plan.ActionUpdate,
			Address: "example.labels",
			Type:    "example",
			Attributes: []plan.AttributeChange{{
				Path:   "labels",
				Before: plan.Value{Kind: plan.ValueRaw, Raw: map[string]any{"team": "infra"}},
				After:  plan.Value{Kind: plan.ValueRaw, Raw: map[string]any{}},
			}},
		}},
	}
	got := RenderCompact(p, Options{Limits: DefaultLimits()})
	if !strings.Contains(got, `labels={"team":"infra"}->{}`) {
		t.Fatalf("meaningful empty transition was dropped: %s", got)
	}
}

func mustParseCompactFixture(t *testing.T, name string) *plan.Plan {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := plan.Parse(data, plan.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
