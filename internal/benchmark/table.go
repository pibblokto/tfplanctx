package benchmark

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// Row is one token-efficiency comparison row.
type Row struct {
	Name           string
	Review         Report
	Detail         Report
	TextPlanReport *Report
}

// WriteTable renders a compact, human-readable token-efficiency table.
func WriteTable(w io.Writer, rows []Row) error {
	tab := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	withText := hasTextPlanRows(rows)
	if withText {
		if _, err := fmt.Fprintln(tab, "fixture\tjson_tokens\ttxt_tokens\treview_tokens\tdetail_tokens\treview_vs_json\treview_vs_txt"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(tab, "fixture\tjson_tokens\treview_tokens\tdetail_tokens\treview_saved\treview_reduction"); err != nil {
			return err
		}
	}
	for _, row := range rows {
		if withText {
			if row.TextPlanReport == nil {
				if _, err := fmt.Fprintf(
					tab,
					"%s\t%d\t-\t%d\t%d\t%.1f%%\t-\n",
					row.Name,
					row.Review.InputTokens,
					row.Review.OutputTokens,
					row.Detail.OutputTokens,
					row.Review.ReductionPercent,
				); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprintf(
				tab,
				"%s\t%d\t%d\t%d\t%d\t%.1f%%\t%.1f%%\n",
				row.Name,
				row.Review.InputTokens,
				row.TextPlanReport.InputTokens,
				row.Review.OutputTokens,
				row.Detail.OutputTokens,
				row.Review.ReductionPercent,
				row.TextPlanReport.ReductionPercent,
			); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(
			tab,
			"%s\t%d\t%d\t%d\t%d\t%.1f%%\n",
			row.Name,
			row.Review.InputTokens,
			row.Review.OutputTokens,
			row.Detail.OutputTokens,
			row.Review.TokensSaved,
			row.Review.ReductionPercent,
		); err != nil {
			return err
		}
	}
	return tab.Flush()
}

func hasTextPlanRows(rows []Row) bool {
	for _, row := range rows {
		if row.TextPlanReport != nil {
			return true
		}
	}
	return false
}
