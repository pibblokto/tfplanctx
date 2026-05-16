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

func TestApproxTokensRoundsUp(t *testing.T) {
	cases := map[int]int{0: 0, 1: 1, 4: 1, 5: 2, 8: 2}
	for chars, want := range cases {
		if got := ApproxTokensFromChars(chars); got != want {
			t.Fatalf("chars=%d tokens=%d, want %d", chars, got, want)
		}
	}
}
