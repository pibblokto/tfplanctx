package benchmark

import (
	"bytes"
	"testing"
)

func TestWriteTable(t *testing.T) {
	var out bytes.Buffer
	err := WriteTable(&out, []Row{{
		Name: "plan.json",
		Report: Report{
			InputTokens:      100,
			OutputTokens:     25,
			TokensSaved:      75,
			ReductionPercent: 75,
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	want := "fixture    raw_tokens  tfp1_tokens  tokens_saved  reduction\nplan.json  100         25           75            75.0%\n"
	if got != want {
		t.Fatalf("table = %q, want %q", got, want)
	}
}
