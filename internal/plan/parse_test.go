package plan_test

import (
	"os"
	"path/filepath"
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
}

func mustParseFixture(t *testing.T, name string) *plan.Plan {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := plan.Parse(data, plan.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return parsed
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
