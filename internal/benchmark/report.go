package benchmark

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Report describes approximate context reduction from one baseline input to rendered output.
type Report struct {
	InputChars       int
	OutputChars      int
	InputTokens      int
	OutputTokens     int
	TokensSaved      int
	ReductionPercent float64
}

// CompactStats is the renderer-provided compression summary attached to benchmark output.
type CompactStats struct {
	Omitted          int
	GroupedResources int
	GroupCount       int
	TemplateCount    int
	DictionaryCount  int
	LensResources    int
	DriftSummarized  int
}

// CompactComparison compares compact review and detail outputs against available baselines.
type CompactComparison struct {
	JSON   Report
	Text   *Report
	Review Report
	Detail Report
	Stats  CompactStats
}

// Compare returns a deterministic approximation based on the common chars/4 token heuristic.
func Compare(input []byte, output string) Report {
	inputChars := utf8.RuneCount(input)
	outputChars := utf8.RuneCountInString(output)
	inputTokens := ApproxTokensFromChars(inputChars)
	outputTokens := ApproxTokensFromChars(outputChars)
	saved := inputTokens - outputTokens

	var reduction float64
	if inputTokens > 0 {
		reduction = float64(saved) / float64(inputTokens) * 100
	}

	return Report{
		InputChars:       inputChars,
		OutputChars:      outputChars,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		TokensSaved:      saved,
		ReductionPercent: reduction,
	}
}

// ApproxTokensFromChars estimates LLM tokens using ceil(chars/4).
func ApproxTokensFromChars(chars int) int {
	if chars <= 0 {
		return 0
	}
	return (chars + 3) / 4
}

// String renders one stable, grepable benchmark line using the JSON/raw input baseline.
func (r Report) String() string {
	return fmt.Sprintf(
		"BENCH approx_tokens_in=%d approx_tokens_out=%d tokens_saved=%d reduction=%.1f%% chars_in=%d chars_out=%d",
		r.InputTokens,
		r.OutputTokens,
		r.TokensSaved,
		r.ReductionPercent,
		r.InputChars,
		r.OutputChars,
	)
}

// StringWithTextPlan adds a second baseline for human-readable Terraform plan output.
func (r Report) StringWithTextPlan(textPlan Report) string {
	return fmt.Sprintf(
		"%s txt_plan_tokens_in=%d txt_plan_tokens_saved=%d txt_plan_reduction=%.1f%% txt_plan_chars_in=%d",
		r.String(),
		textPlan.InputTokens,
		textPlan.TokensSaved,
		textPlan.ReductionPercent,
		textPlan.InputChars,
	)
}

// CompareCompact builds a two-output benchmark report for compact review/detail modes.
func CompareCompact(jsonInput []byte, textPlan []byte, reviewOutput, detailOutput string, stats CompactStats) CompactComparison {
	review := Compare(jsonInput, reviewOutput)
	detail := Compare(jsonInput, detailOutput)
	comparison := CompactComparison{
		JSON:   Report{InputChars: review.InputChars, InputTokens: review.InputTokens},
		Review: review,
		Detail: detail,
		Stats:  stats,
	}
	if textPlan != nil {
		textReview := Compare(textPlan, reviewOutput)
		comparison.Text = &textReview
	}
	return comparison
}

// String renders one stable compact-mode benchmark line.
func (c CompactComparison) String() string {
	parts := []string{
		fmt.Sprintf("BENCH json_tokens=%d", c.JSON.InputTokens),
		fmt.Sprintf("json_chars=%d", c.JSON.InputChars),
	}
	if c.Text != nil {
		parts = append(parts,
			fmt.Sprintf("txt_tokens=%d", c.Text.InputTokens),
			fmt.Sprintf("txt_chars=%d", c.Text.InputChars),
		)
	}
	parts = append(parts,
		fmt.Sprintf("review_tokens=%d", c.Review.OutputTokens),
		fmt.Sprintf("review_chars=%d", c.Review.OutputChars),
		fmt.Sprintf("detail_tokens=%d", c.Detail.OutputTokens),
		fmt.Sprintf("detail_chars=%d", c.Detail.OutputChars),
		fmt.Sprintf("review_vs_json_reduction=%.1f%%", c.Review.ReductionPercent),
		fmt.Sprintf("detail_vs_json_reduction=%.1f%%", c.Detail.ReductionPercent),
	)
	if c.Text != nil {
		parts = append(parts,
			fmt.Sprintf("review_vs_txt_reduction=%.1f%%", c.Text.ReductionPercent),
			fmt.Sprintf("detail_vs_txt_reduction=%.1f%%", reduction(c.Text.InputTokens, c.Detail.OutputTokens)),
		)
	}
	parts = append(parts,
		fmt.Sprintf("omitted=%d", c.Stats.Omitted),
		fmt.Sprintf("grouped_resources=%d", c.Stats.GroupedResources),
		fmt.Sprintf("groups=%d", c.Stats.GroupCount),
		fmt.Sprintf("lens_resources=%d", c.Stats.LensResources),
		fmt.Sprintf("templates=%d", c.Stats.TemplateCount),
		fmt.Sprintf("dict_values=%d", c.Stats.DictionaryCount),
		fmt.Sprintf("drift_summarized=%d", c.Stats.DriftSummarized),
	)
	return strings.Join(parts, " ")
}

func reduction(inputTokens, outputTokens int) float64 {
	if inputTokens == 0 {
		return 0
	}
	return float64(inputTokens-outputTokens) / float64(inputTokens) * 100
}
