package benchmark

import (
	"bytes"
	"testing"
)

func TestWriteTable(t *testing.T) {
	var out bytes.Buffer
	err := WriteTable(&out, []Row{{
		Name: "plan.json",
		Review: Report{
			InputTokens:      100,
			OutputTokens:     25,
			TokensSaved:      75,
			ReductionPercent: 75,
		},
		Detail: Report{OutputTokens: 40},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	want := "fixture    json_tokens  review_tokens  detail_tokens  review_saved  review_reduction\nplan.json  100          25             40             75            75.0%\n"
	if got != want {
		t.Fatalf("table = %q, want %q", got, want)
	}
}

func TestWriteTableAddsTextPlanColumnsWhenPresent(t *testing.T) {
	var out bytes.Buffer
	textReport := Report{
		InputTokens:      60,
		OutputTokens:     25,
		TokensSaved:      35,
		ReductionPercent: 58.3,
	}
	err := WriteTable(&out, []Row{{
		Name: "plan.json",
		Review: Report{
			InputTokens:      100,
			OutputTokens:     25,
			TokensSaved:      75,
			ReductionPercent: 75,
		},
		Detail:         Report{OutputTokens: 40},
		TextPlanReport: &textReport,
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	want := "fixture    json_tokens  txt_tokens  review_tokens  detail_tokens  review_vs_json  review_vs_txt\nplan.json  100          60          25             40             75.0%           58.3%\n"
	if got != want {
		t.Fatalf("table = %q, want %q", got, want)
	}
}
