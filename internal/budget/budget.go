package budget

import (
	"fmt"

	"github.com/piblokto/tfplanctx/internal/plan"
	"github.com/piblokto/tfplanctx/internal/render"
)

// Fit progressively compresses output until it fits the approximate character budget.
func Fit(p *plan.Plan, format string, base render.Options, maxChars int) (string, render.Options, error) {
	if maxChars <= 0 {
		output, err := render.Render(format, p, base)
		return output, base, err
	}

	baseline := base
	baselineCount := render.RecordCount(p, baseline)
	candidates := []render.Options{baseline}

	if !base.Summary {
		candidates = append(candidates,
			withLimits(base, shrink(base.Limits, 2)),
			withLimits(base, shrink(base.Limits, 4)),
		)
		summary := base
		summary.Summary = true
		candidates = append(candidates, summary)
	}

	essential := base
	essential.Summary = true
	essential.EssentialOnly = true
	candidates = append(candidates, essential)

	for _, candidate := range candidates {
		candidate.Omitted = omittedCount(p, baselineCount, candidate)
		output, err := render.Render(format, p, candidate)
		if err != nil {
			return "", candidate, err
		}
		if len(output) <= maxChars {
			return output, candidate, nil
		}
	}

	headerOnly := base
	headerOnly.Summary = true
	headerOnly.EssentialOnly = true
	headerOnly.HeaderOnly = true
	headerOnly.Omitted = baselineCount
	output, err := render.Render(format, p, headerOnly)
	if err != nil {
		return "", headerOnly, err
	}
	if output == "" {
		return "", headerOnly, fmt.Errorf("rendered empty output under budget")
	}
	return output, headerOnly, nil
}

func withLimits(opts render.Options, limits render.Limits) render.Options {
	opts.Limits = limits
	return opts
}

func shrink(limits render.Limits, factor int) render.Limits {
	if limits.MaxValueLen == 0 {
		limits = render.DefaultLimits()
	}
	return render.Limits{
		MaxValueLen:   max(24, limits.MaxValueLen/factor),
		MaxListItems:  max(3, limits.MaxListItems/factor),
		MaxObjectKeys: max(5, limits.MaxObjectKeys/factor),
	}
}

func omittedCount(p *plan.Plan, baselineCount int, candidate render.Options) int {
	if candidate.HeaderOnly {
		return baselineCount
	}
	candidateCount := render.RecordCount(p, candidate)
	if candidateCount >= baselineCount {
		return 0
	}
	return baselineCount - candidateCount
}
