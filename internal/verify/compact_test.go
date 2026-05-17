package verify_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pibblokto/tfplanctx/internal/plan"
	"github.com/pibblokto/tfplanctx/internal/render"
	"github.com/pibblokto/tfplanctx/internal/verify"
)

func TestCompactVerifierRejectsMissingResource(t *testing.T) {
	parsed := mustParseFixture(t, "plan_tfp2_coverage.json")
	output := render.RenderCompact(parsed, render.Options{Limits: render.DefaultLimits()})
	output = strings.Replace(output, lineContaining(output, "module.foo.terraform_data.validation")+"\n", "", 1)
	if err := verify.Compact(parsed, output); err == nil || (!strings.Contains(err.Error(), "missing resource") && !strings.Contains(err.Error(), "resource record count")) {
		t.Fatalf("err = %v", err)
	}
}

func TestCompactVerifierAcceptsValidOutput(t *testing.T) {
	parsed := mustParseFixture(t, "plan_tfp2_coverage.json")
	output := render.RenderCompact(parsed, render.Options{Limits: render.DefaultLimits()})
	if err := verify.Compact(parsed, output); err != nil {
		t.Fatal(err)
	}
}

func TestCompactVerifierAcceptsGroupedTemplateOutput(t *testing.T) {
	parsed := mustParseFixture(t, "plan_repetitive.json")
	output := render.RenderCompact(parsed, render.Options{Limits: render.DefaultLimits()})
	if !strings.Contains(output, "GL|G1|") || !strings.Contains(output, "TPL|P1|") {
		t.Fatalf("fixture did not exercise grouped/template output:\n%s", output)
	}
	if err := verify.Compact(parsed, output); err != nil {
		t.Fatal(err)
	}
}

func TestCompactVerifierAcceptsIAMLensDictionaryOutput(t *testing.T) {
	parsed := mustParseFixture(t, "plan_lens_dictionary.json")
	output := render.RenderCompact(parsed, render.Options{Limits: render.DefaultLimits()})
	if !strings.Contains(output, "L|IAM|") || !strings.Contains(output, "VAL|") {
		t.Fatalf("fixture did not exercise IAM lens/dictionary output:\n%s", output)
	}
	if err := verify.Compact(parsed, output); err != nil {
		t.Fatal(err)
	}
}

func TestCompactVerifierRejectsUnknownDictionaryReference(t *testing.T) {
	parsed := mustParseFixture(t, "plan_nested.json")
	output := render.RenderCompact(parsed, render.Options{Limits: render.DefaultLimits()})
	output = strings.Replace(output, `settings.labels.tier="web"->"api"`, `settings.labels.tier=$V9`, 1)
	if err := verify.Compact(parsed, output); err == nil || !strings.Contains(err.Error(), "unknown value dictionary") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompactVerifierRejectsMissingOutput(t *testing.T) {
	parsed := mustParseFixture(t, "plan_output_changes.json")
	output := render.RenderCompact(parsed, render.Options{Limits: render.DefaultLimits()})
	output = strings.Replace(output, lineContaining(output, `O|updated|`)+"\n", "", 1)
	if err := verify.Compact(parsed, output); err == nil || (!strings.Contains(err.Error(), "missing output") && !strings.Contains(err.Error(), "output record count")) {
		t.Fatalf("err = %v", err)
	}
}

func TestCompactVerifierRejectsOmissionAccountingMismatch(t *testing.T) {
	parsed := mustParseFixture(t, "plan_lens_dictionary.json")
	output := render.RenderCompact(parsed, render.Options{Limits: render.DefaultLimits()})
	output = strings.Replace(output, "OMITTED=8", "OMITTED=9", 1)
	if err := verify.Compact(parsed, output); err == nil || !strings.Contains(err.Error(), "omitted header") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompactVerifierRejectsMissingReplacementValueDetail(t *testing.T) {
	parsed := mustParseFixture(t, "plan_lens_dictionary.json")
	output := render.RenderCompact(parsed, render.Options{Limits: render.DefaultLimits()})
	output = strings.Replace(
		output,
		`R|google_artifact_registry_repository.deepsearch_dev_repository|repository_id="deepsearch-backend-repository"->"deepsearch-backend-repository2"|acts=delete,create;repl=repository_id`,
		`R|google_artifact_registry_repository.deepsearch_dev_repository||attrs=none;acts=delete,create;repl=repository_id`,
		1,
	)
	if err := verify.Compact(parsed, output); err == nil || !strings.Contains(err.Error(), "replace_path") {
		t.Fatalf("err = %v", err)
	}
}

func lineContaining(output, needle string) string {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func mustParseFixture(t *testing.T, name string) *plan.Plan {
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
