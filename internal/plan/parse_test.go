package plan_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piblokto/tfplanctx/internal/plan"
	"github.com/piblokto/tfplanctx/internal/redact"
)

func TestParseMainFixture(t *testing.T) {
	parsed := mustParseFixture(t, "plan_main.json")

	if got, want := parsed.Summary.Creates, 1; got != want {
		t.Fatalf("creates = %d, want %d", got, want)
	}
	if got, want := parsed.Summary.Updates, 7; got != want {
		t.Fatalf("updates = %d, want %d", got, want)
	}
	if got, want := parsed.Summary.Replaces, 1; got != want {
		t.Fatalf("replaces = %d, want %d", got, want)
	}
	if got, want := parsed.Summary.Deletes, 1; got != want {
		t.Fatalf("deletes = %d, want %d", got, want)
	}
	if got, want := parsed.Summary.OutputChanges, 1; got != want {
		t.Fatalf("outputs = %d, want %d", got, want)
	}
	if got, want := parsed.Summary.RiskResources, 5; got != want {
		t.Fatalf("risk resources = %d, want %d", got, want)
	}

	bucket := findResource(t, parsed, "aws_s3_bucket.logs")
	if len(bucket.Attributes) != 1 || bucket.Attributes[0].Path != "bucket" {
		t.Fatalf("bucket attrs = %#v", bucket.Attributes)
	}
	if bucket.Attributes[0].Before.Kind != plan.ValueNull {
		t.Fatalf("bucket before kind = %s", bucket.Attributes[0].Before.Kind)
	}

	unknown := findResource(t, parsed, "aws_instance.unknown")
	if got := unknown.Attributes[0].After.Kind; got != plan.ValueUnknown {
		t.Fatalf("unknown after kind = %s", got)
	}

	replace := findResource(t, parsed, "aws_instance.app")
	if got := replace.Attributes[0].Flags; len(got) != 1 || got[0] != "replace_path" {
		t.Fatalf("replace flags = %#v", got)
	}

	secret := findResource(t, parsed, "example_service.secret")
	password := findAttribute(t, secret, "password")
	if password.Before.Kind != plan.ValueSensitive || password.After.Kind != plan.ValueSensitive {
		t.Fatalf("terraform sensitive values were not redacted: %#v", password)
	}

	heuristic := findResource(t, parsed, "example_service.heuristic")
	if heuristic.Attributes[0].Before.Kind != plan.ValueSensitive || heuristic.Attributes[0].After.Kind != plan.ValueSensitive {
		t.Fatalf("heuristic sensitive values were not redacted: %#v", heuristic.Attributes[0])
	}

	deleted := findResource(t, parsed, "aws_db_instance.old")
	if len(deleted.Risks) != 1 || deleted.Risks[0].Name != "data_loss" {
		t.Fatalf("delete risks = %#v", deleted.Risks)
	}
}

func TestNestedDiffPaths(t *testing.T) {
	parsed := mustParseFixture(t, "plan_nested.json")
	resource := findResource(t, parsed, "example_app.main")
	if got, want := len(resource.Attributes), 2; got != want {
		t.Fatalf("attribute count = %d, want %d", got, want)
	}
	if got, want := resource.Attributes[0].Path, "settings.labels.tier"; got != want {
		t.Fatalf("first path = %q, want %q", got, want)
	}
	if got, want := resource.Attributes[1].Path, "settings.replicas"; got != want {
		t.Fatalf("second path = %q, want %q", got, want)
	}
}

func TestUnsafeSensitiveStillKeepsHeuristics(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "plan_main.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := plan.Parse(data, plan.ParseOptions{
		Redact: redact.Config{UnsafeShowSensitive: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	terraformSensitive := findResource(t, parsed, "example_service.secret")
	configValue := findAttribute(t, terraformSensitive, "config_value")
	if configValue.Before.Kind != plan.ValueRaw {
		t.Fatalf("terraform-marked sensitive value should be visible with unsafe override: %#v", configValue)
	}
	password := findAttribute(t, terraformSensitive, "password")
	if password.Before.Kind != plan.ValueSensitive {
		t.Fatalf("heuristic redaction should still protect password paths: %#v", password)
	}
	heuristic := findResource(t, parsed, "example_service.heuristic")
	if heuristic.Attributes[0].Before.Kind != plan.ValueSensitive {
		t.Fatalf("heuristic redaction should remain active: %#v", heuristic.Attributes[0])
	}
}

func TestOrderingIsDeterministic(t *testing.T) {
	parsed := mustParseFixture(t, "plan_main.json")
	got := []string{
		parsed.Resources[0].Address,
		parsed.Resources[1].Address,
		parsed.Resources[2].Address,
		parsed.Resources[3].Address,
	}
	want := []string{
		"aws_db_instance.old",
		"aws_instance.app",
		"aws_s3_bucket.logs",
		"aws_iam_policy.admin",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resource order[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResourceFilter(t *testing.T) {
	parsed := mustParseFixture(t, "plan_main.json")
	filtered := parsed.Filter("aws_security_group.web", "")
	if len(filtered.Resources) != 1 || filtered.Resources[0].Address != "aws_security_group.web" {
		t.Fatalf("filtered resources = %#v", filtered.Resources)
	}
	if got, want := filtered.Summary.Updates, 1; got != want {
		t.Fatalf("filtered updates = %d, want %d", got, want)
	}
	if got, want := filtered.Summary.RiskResources, 1; got != want {
		t.Fatalf("filtered risks = %d, want %d", got, want)
	}
}

func TestTypeFilter(t *testing.T) {
	parsed := mustParseFixture(t, "plan_main.json")
	filtered := parsed.Filter("", "aws_instance")
	if got, want := len(filtered.Resources), 2; got != want {
		t.Fatalf("filtered resources = %d, want %d", got, want)
	}
	if got, want := filtered.Summary.Updates, 1; got != want {
		t.Fatalf("filtered updates = %d, want %d", got, want)
	}
	if got, want := filtered.Summary.Replaces, 1; got != want {
		t.Fatalf("filtered replaces = %d, want %d", got, want)
	}
}

func TestReadChangesAreRepresentedAndNoOpsAreTracked(t *testing.T) {
	data := mustReadFixture(t, "plan_read_noop.json")
	parsed, err := plan.Parse(data, plan.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(parsed.Resources), 1; got != want {
		t.Fatalf("resources = %d, want %d", got, want)
	}
	if got, want := parsed.Resources[0].Address, "data.aws_ami.ubuntu"; got != want {
		t.Fatalf("read address = %q, want %q", got, want)
	}
	if got, want := parsed.Resources[0].Action, plan.ActionRead; got != want {
		t.Fatalf("read action = %q, want %q", got, want)
	}
	if got, want := len(parsed.NoOpResources), 1; got != want {
		t.Fatalf("noop resources = %d, want %d", got, want)
	}
}

func TestDirectTerraformOutputChangesAreNormalized(t *testing.T) {
	parsed := mustParseFixture(t, "plan_output_changes.json")
	if got, want := parsed.Summary.OutputChanges, 7; got != want {
		t.Fatalf("outputs = %d, want %d", got, want)
	}
	if got, want := parsed.Outputs[0].Name, "complex_output"; got != want {
		t.Fatalf("first output name = %q, want %q", got, want)
	}
	unknown := findOutput(t, parsed, "output.unknown_after")
	if got, want := unknown.Attributes[0].After.Kind, plan.ValueUnknown; got != want {
		t.Fatalf("unknown output after = %s, want %s", got, want)
	}
	if got, want := unknown.UnknownPaths, []string{"value"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("unknown output paths = %#v, want %#v", got, want)
	}
	sensitive := findOutput(t, parsed, "output.sensitive_output")
	if got, want := sensitive.Attributes[0].Before.Kind, plan.ValueSensitive; got != want {
		t.Fatalf("sensitive output before = %s, want %s", got, want)
	}
	if got, want := sensitive.SensitivePaths, []string{"value"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("sensitive output paths = %#v, want %#v", got, want)
	}
}

func TestFixtureJSONFilesHaveHumanReadablePairs(t *testing.T) {
	entries, err := filepath.Glob(filepath.Join("..", "..", "testdata", "plan_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected JSON fixtures")
	}
	for _, jsonPath := range entries {
		textPath := strings.TrimSuffix(jsonPath, ".json") + ".tfplan.txt"
		if _, err := os.Stat(textPath); err != nil {
			t.Fatalf("fixture %s is missing paired Terraform text output %s: %v", jsonPath, textPath, err)
		}
	}
}

func TestMalformedJSONReturnsHelpfulError(t *testing.T) {
	_, err := plan.Parse([]byte(`{"resource_changes": [`), plan.ParseOptions{})
	if err == nil || !strings.Contains(err.Error(), "parse terraform JSON plan") {
		t.Fatalf("err = %v", err)
	}
}

func mustParseFixture(t *testing.T, name string) *plan.Plan {
	t.Helper()
	data := mustReadFixture(t, name)
	parsed, err := plan.Parse(data, plan.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func findAttribute(t *testing.T, resource plan.ResourceChange, path string) plan.AttributeChange {
	t.Helper()
	for _, attribute := range resource.Attributes {
		if attribute.Path == path {
			return attribute
		}
	}
	t.Fatalf("attribute %s not found on %s", path, resource.Address)
	return plan.AttributeChange{}
}

func findOutput(t *testing.T, parsed *plan.Plan, address string) plan.OutputChange {
	t.Helper()
	for _, output := range parsed.Outputs {
		if output.Address == address {
			return output
		}
	}
	t.Fatalf("missing output %s", address)
	return plan.OutputChange{}
}

func findResource(t *testing.T, parsed *plan.Plan, address string) plan.ResourceChange {
	t.Helper()
	for _, resource := range parsed.Resources {
		if resource.Address == address {
			return resource
		}
	}
	t.Fatalf("resource %s not found", address)
	return plan.ResourceChange{}
}
