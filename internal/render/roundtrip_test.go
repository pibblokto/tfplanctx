package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/piblokto/tfplanctx/internal/plan"
)

func TestLineOutputMatchesNormalizedPlanChanges(t *testing.T) {
	parsed := mustParseFixture(t, "plan_main.json")
	limits := DefaultLimits()
	got := parseLineRecords(t, RenderLine(parsed, Options{Limits: limits}))
	want := expectedLineRecords(parsed, limits)

	if len(got) != len(want) {
		t.Fatalf("record count = %d, want %d\nrecords=%#v", len(got), len(want), got)
	}
	for key, expected := range want {
		actual, ok := got[key]
		if !ok {
			t.Fatalf("missing rendered change %s", key)
		}
		if actual != expected {
			t.Fatalf("record %s = %#v, want %#v", key, actual, expected)
		}
	}
}

func TestRiskOnlyOutputIsSubsetOfFullOutput(t *testing.T) {
	parsed := mustParseFixture(t, "plan_main.json")
	full := parseLineRecords(t, RenderLine(parsed, Options{Limits: DefaultLimits()}))
	riskOnly := parseLineRecords(t, RenderLine(parsed, Options{RiskOnly: true, Limits: DefaultLimits()}))
	if len(riskOnly) == 0 {
		t.Fatal("expected risky records")
	}
	for key, record := range riskOnly {
		fullRecord, ok := full[key]
		if !ok || fullRecord != record {
			t.Fatalf("risk-only record %s is not present in full output", key)
		}
		if !strings.Contains(record.flags, "risk=") {
			t.Fatalf("risk-only record is missing risk flag: %#v", record)
		}
	}
}

type lineRecord struct {
	before string
	after  string
	flags  string
}

func parseLineRecords(t *testing.T, output string) map[string]lineRecord {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "TFP1 ") {
		t.Fatalf("missing line header: %q", output)
	}
	records := make(map[string]lineRecord)
	for _, line := range lines[1:] {
		parts := strings.SplitN(line, "|", 6)
		if len(parts) != 6 {
			t.Fatalf("invalid line record %q", line)
		}
		key := strings.Join(parts[:3], "|")
		records[key] = lineRecord{before: parts[3], after: parts[4], flags: parts[5]}
	}
	return records
}

func expectedLineRecords(parsed *plan.Plan, limits Limits) map[string]lineRecord {
	records := make(map[string]lineRecord)
	for _, resource := range parsed.Resources {
		for _, attribute := range resource.Attributes {
			key := strings.Join([]string{string(resource.Action), resource.Address, attribute.Path}, "|")
			records[key] = lineRecord{
				before: lineValue(attribute.Before, limits),
				after:  lineValue(attribute.After, limits),
				flags:  strings.Join(attribute.Flags, ","),
			}
		}
	}
	for _, output := range parsed.Outputs {
		for _, attribute := range output.Attributes {
			key := strings.Join([]string{string(plan.ActionOutput), output.Address, attribute.Path}, "|")
			records[key] = lineRecord{
				before: lineValue(attribute.Before, limits),
				after:  lineValue(attribute.After, limits),
				flags:  strings.Join(attribute.Flags, ","),
			}
		}
	}
	return records
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
