package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/piblokto/tfplanctx/internal/plan"
	"github.com/piblokto/tfplanctx/internal/render"
)

type rawParityPlan struct {
	ResourceChanges []struct {
		Address string `json:"address"`
		Change  struct {
			Actions []string `json:"actions"`
		} `json:"change"`
	} `json:"resource_changes"`
	OutputChanges map[string]struct {
		Actions []string `json:"actions"`
		Change  struct {
			Actions []string `json:"actions"`
		} `json:"change"`
	} `json:"output_changes"`
}

func TestCompactOutputMatchesRawTerraformResourceChanges(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "plan_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("expected plan fixtures")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			assertCompactMatchesRawPlan(t, path)
		})
	}
}

func assertCompactMatchesRawPlan(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw rawParityPlan
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	parsed, err := plan.Parse(data, plan.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	output := render.RenderCompact(parsed, render.Options{Limits: render.DefaultLimits()})
	compact, err := parseCompact(output)
	if err != nil {
		t.Fatal(err)
	}

	wantAddresses := make(map[string]struct{})
	wantCounts := map[string]int{}
	for _, resource := range raw.ResourceChanges {
		if len(resource.Change.Actions) == 1 && resource.Change.Actions[0] == "no-op" {
			continue
		}
		wantAddresses[resource.Address] = struct{}{}
		switch {
		case len(resource.Change.Actions) == 1 && resource.Change.Actions[0] == "create":
			wantCounts["C"]++
		case len(resource.Change.Actions) == 1 && resource.Change.Actions[0] == "update":
			wantCounts["U"]++
		case len(resource.Change.Actions) == 1 && resource.Change.Actions[0] == "delete":
			wantCounts["D"]++
		case len(resource.Change.Actions) == 1 && resource.Change.Actions[0] == "read":
			wantCounts["Q"]++
		case len(resource.Change.Actions) == 2:
			wantCounts["R"]++
		}
	}
	if got, want := len(compact.Resources), len(wantAddresses); got != want {
		t.Fatalf("rendered resource count = %d, want %d", got, want)
	}
	for address := range wantAddresses {
		if !compactHasAddress(compact.Resources, address) {
			t.Fatalf("raw changed resource %s missing from compact output", address)
		}
	}
	for action, want := range wantCounts {
		if got := compact.Header[action]; got != want {
			t.Fatalf("header %s=%d, want %d", action, got, want)
		}
	}
	wantOutputs := 0
	for _, output := range raw.OutputChanges {
		actions := output.Actions
		if len(output.Change.Actions) > 0 {
			actions = output.Change.Actions
		}
		if len(actions) == 1 && actions[0] == "no-op" {
			continue
		}
		wantOutputs++
	}
	if got, want := len(compact.Outputs), wantOutputs; got != want {
		t.Fatalf("rendered output count = %d, want %d", got, want)
	}
}

func compactHasAddress(records map[string]compactRecord, address string) bool {
	for _, record := range records {
		if record.Addr == address {
			return true
		}
	}
	return false
}
