package benchmark

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// Row is one token-efficiency comparison row.
type Row struct {
	Name   string
	Report Report
}

// WriteTable renders a compact, human-readable token-efficiency table.
func WriteTable(w io.Writer, rows []Row) error {
	tab := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tab, "fixture\traw_tokens\ttfp1_tokens\ttokens_saved\treduction"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(
			tab,
			"%s\t%d\t%d\t%d\t%.1f%%\n",
			row.Name,
			row.Report.InputTokens,
			row.Report.OutputTokens,
			row.Report.TokensSaved,
			row.Report.ReductionPercent,
		); err != nil {
			return err
		}
	}
	return tab.Flush()
}
