package benchmark

import (
	"fmt"
	"unicode/utf8"
)

// Report describes approximate context reduction from raw JSON input to rendered output.
type Report struct {
	InputChars       int
	OutputChars      int
	InputTokens      int
	OutputTokens     int
	TokensSaved      int
	ReductionPercent float64
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

// String renders one stable, grepable benchmark line.
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
