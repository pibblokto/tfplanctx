package benchmark

import "testing"

func TestCompareReportsApproximateSavings(t *testing.T) {
	report := Compare([]byte("abcdefghijklmnop"), "abcd")
	if got, want := report.InputTokens, 4; got != want {
		t.Fatalf("input tokens = %d, want %d", got, want)
	}
	if got, want := report.OutputTokens, 1; got != want {
		t.Fatalf("output tokens = %d, want %d", got, want)
	}
	if got, want := report.TokensSaved, 3; got != want {
		t.Fatalf("tokens saved = %d, want %d", got, want)
	}
	if got, want := report.ReductionPercent, 75.0; got != want {
		t.Fatalf("reduction = %.1f, want %.1f", got, want)
	}
}

func TestReportStringIsStable(t *testing.T) {
	report := Report{InputChars: 16, OutputChars: 4, InputTokens: 4, OutputTokens: 1, TokensSaved: 3, ReductionPercent: 75}
	got := report.String()
	want := "BENCH approx_tokens_in=4 approx_tokens_out=1 tokens_saved=3 reduction=75.0% chars_in=16 chars_out=4"
	if got != want {
		t.Fatalf("report string = %q, want %q", got, want)
	}
}

func TestReportStringWithTextPlanAddsSecondBaseline(t *testing.T) {
	jsonReport := Report{InputChars: 16, OutputChars: 4, InputTokens: 4, OutputTokens: 1, TokensSaved: 3, ReductionPercent: 75}
	textPlanReport := Report{InputChars: 12, OutputChars: 4, InputTokens: 3, OutputTokens: 1, TokensSaved: 2, ReductionPercent: 66.7}
	got := jsonReport.StringWithTextPlan(textPlanReport)
	want := "BENCH approx_tokens_in=4 approx_tokens_out=1 tokens_saved=3 reduction=75.0% chars_in=16 chars_out=4 txt_plan_tokens_in=3 txt_plan_tokens_saved=2 txt_plan_reduction=66.7% txt_plan_chars_in=12"
	if got != want {
		t.Fatalf("report string = %q, want %q", got, want)
	}
}

func TestCompactComparisonStringIncludesReviewAndDetailMetrics(t *testing.T) {
	comparison := CompareCompact(
		[]byte("abcdefghijklmnop"),
		[]byte("abcdefghijkl"),
		"abcd",
		"abcdefgh",
		CompactStats{Omitted: 2, GroupedResources: 3, GroupCount: 1, TemplateCount: 1, DriftSummarized: 4},
	)
	got := comparison.String()
	want := "BENCH json_tokens=4 json_chars=16 txt_tokens=3 txt_chars=12 review_tokens=1 review_chars=4 detail_tokens=2 detail_chars=8 review_vs_json_reduction=75.0% detail_vs_json_reduction=50.0% review_vs_txt_reduction=66.7% detail_vs_txt_reduction=33.3% omitted=2 grouped_resources=3 groups=1 lens_resources=0 templates=1 dict_values=0 drift_summarized=4"
	if got != want {
		t.Fatalf("compact benchmark = %q, want %q", got, want)
	}
}

func TestApproxTokensRoundsUp(t *testing.T) {
	cases := map[int]int{0: 0, 1: 1, 4: 1, 5: 2, 8: 2}
	for chars, want := range cases {
		if got := ApproxTokensFromChars(chars); got != want {
			t.Fatalf("chars=%d tokens=%d, want %d", chars, got, want)
		}
	}
}
