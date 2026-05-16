package input

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadJSONFileDoesNotNeedTerraform(t *testing.T) {
	t.Setenv("PATH", "")
	path := filepath.Join("..", "..", "testdata", "plan_nested.json")
	data, err := Load(context.Background(), path, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "example_app.main") {
		t.Fatalf("unexpected fixture content: %s", string(data))
	}
}

func TestLoadStdinJSON(t *testing.T) {
	data, err := Load(context.Background(), "-", strings.NewReader(`{"resource_changes":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"resource_changes":[]}`; got != want {
		t.Fatalf("stdin data = %q, want %q", got, want)
	}
}

func TestLoadRejectsInvalidStdinJSON(t *testing.T) {
	_, err := Load(context.Background(), "-", strings.NewReader("not-json"))
	if err == nil || !strings.Contains(err.Error(), "valid Terraform JSON") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejectsHumanReadablePlanOutput(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "plan_main.tfplan.txt")
	_, err := Load(context.Background(), path, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "human-readable Terraform plan output") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadBinaryPlanUsesTerraformShow(t *testing.T) {
	tmp := t.TempDir()
	binaryPlan := filepath.Join(tmp, "tfplan")
	if err := os.WriteFile(binaryPlan, []byte("binary-plan"), 0o600); err != nil {
		t.Fatal(err)
	}

	terraform := filepath.Join(tmp, "terraform")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" != \"show\" ] || [ \"$2\" != \"-json\" ]; then exit 9; fi\n" +
		"printf '{\"resource_changes\":[]}'\n"
	if err := os.WriteFile(terraform, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)

	data, err := Load(context.Background(), binaryPlan, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"resource_changes":[]}`; got != want {
		t.Fatalf("terraform output = %q, want %q", got, want)
	}
}
