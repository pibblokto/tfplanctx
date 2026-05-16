package render

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/piblokto/tfplanctx/internal/plan"
)

// Limits control deterministic value summarization.
type Limits struct {
	MaxValueLen   int
	MaxListItems  int
	MaxObjectKeys int
}

// DefaultLimits returns the documented renderer defaults.
func DefaultLimits() Limits {
	return Limits{MaxValueLen: 160, MaxListItems: 20, MaxObjectKeys: 30}
}

// Options control presentation without changing the normalized model.
type Options struct {
	Summary       bool
	RiskOnly      bool
	Detail        bool
	EssentialOnly bool
	HeaderOnly    bool
	IncludeNoOp   bool
	MetadataOnly  bool
	NoGroups      bool
	Omitted       int
	Limits        Limits
}

// Render emits one of the supported output formats.
func Render(format string, p *plan.Plan, opts Options) (string, error) {
	if opts.Limits.MaxValueLen == 0 {
		opts.Limits = DefaultLimits()
	}
	switch format {
	case "compact":
		return RenderCompact(p, opts), nil
	case "line":
		return RenderLine(p, opts), nil
	case "jsonl":
		return RenderJSONL(p, opts)
	case "markdown":
		return RenderMarkdown(p, opts), nil
	default:
		return "", fmt.Errorf("unsupported format %q", format)
	}
}

// RecordCount returns the number of emitted data records excluding the line header.
func RecordCount(p *plan.Plan, opts Options) int {
	if opts.HeaderOnly {
		return 0
	}
	count := 0
	if opts.Summary {
		count += len(selectedResources(p, opts))
		if !opts.EssentialOnly && !opts.RiskOnly {
			count += len(p.Outputs)
		}
		if opts.IncludeNoOp {
			count += len(p.NoOpResources)
		}
		return count
	}
	for _, resource := range selectedResources(p, opts) {
		count += len(resource.Attributes)
	}
	if !opts.EssentialOnly && !opts.RiskOnly {
		for _, output := range p.Outputs {
			count += len(output.Attributes)
		}
	}
	return count
}

// RenderLine emits the compact line protocol.
func RenderLine(p *plan.Plan, opts Options) string {
	var b strings.Builder
	b.WriteString(headerLine(p.Summary, opts.Omitted))
	b.WriteByte('\n')
	if opts.HeaderOnly {
		return b.String()
	}
	if opts.Summary {
		for _, resource := range selectedResources(p, opts) {
			fmt.Fprintf(&b, "%s|%s|changes=%d|%s\n", resource.Action, resource.Address, len(resource.Attributes), strings.Join(resource.SummaryFlags(), ","))
		}
		if !opts.EssentialOnly && !opts.RiskOnly {
			for _, output := range p.Outputs {
				fmt.Fprintf(&b, "%s|%s|changes=%d|\n", plan.ActionOutput, output.Address, len(output.Attributes))
			}
		}
		if opts.IncludeNoOp {
			for _, resource := range p.NoOpResources {
				fmt.Fprintf(&b, "%s|%s|changes=0|\n", resource.Action, resource.Address)
			}
		}
		return b.String()
	}

	for _, resource := range selectedResources(p, opts) {
		if len(resource.Attributes) == 0 {
			fmt.Fprintf(&b, "%s|%s|self|null|null|attrs=none\n", resource.Action, resource.Address)
			continue
		}
		for _, attribute := range resource.Attributes {
			fmt.Fprintf(&b, "%s|%s|%s|%s|%s|%s\n",
				resource.Action,
				resource.Address,
				attribute.Path,
				lineValue(attribute.Before, opts.Limits),
				lineValue(attribute.After, opts.Limits),
				strings.Join(attribute.Flags, ","),
			)
		}
	}
	if !opts.EssentialOnly && !opts.RiskOnly {
		for _, output := range p.Outputs {
			for _, attribute := range output.Attributes {
				fmt.Fprintf(&b, "%s|%s|%s|%s|%s|%s\n",
					plan.ActionOutput,
					output.Address,
					attribute.Path,
					lineValue(attribute.Before, opts.Limits),
					lineValue(attribute.After, opts.Limits),
					strings.Join(attribute.Flags, ","),
				)
			}
		}
	}
	return b.String()
}

// RenderJSONL emits one JSON object per emitted record.
func RenderJSONL(p *plan.Plan, opts Options) (string, error) {
	var lines []string
	if opts.HeaderOnly {
		header, err := marshalJSONLine(summaryObject(p.Summary, opts.Omitted))
		if err != nil {
			return "", err
		}
		return header + "\n", nil
	}
	if opts.Summary {
		for _, resource := range selectedResources(p, opts) {
			line, err := marshalJSONLine(map[string]any{
				"a":       string(resource.Action),
				"addr":    resource.Address,
				"changes": len(resource.Attributes),
				"flags":   resource.SummaryFlags(),
			})
			if err != nil {
				return "", err
			}
			lines = append(lines, line)
		}
		if !opts.EssentialOnly && !opts.RiskOnly {
			for _, output := range p.Outputs {
				line, err := marshalJSONLine(map[string]any{
					"a":       string(plan.ActionOutput),
					"addr":    output.Address,
					"changes": len(output.Attributes),
					"flags":   []string{},
				})
				if err != nil {
					return "", err
				}
				lines = append(lines, line)
			}
		}
		if opts.IncludeNoOp {
			for _, resource := range p.NoOpResources {
				line, err := marshalJSONLine(map[string]any{
					"a":       string(resource.Action),
					"addr":    resource.Address,
					"changes": 0,
					"flags":   []string{},
				})
				if err != nil {
					return "", err
				}
				lines = append(lines, line)
			}
		}
	} else {
		for _, resource := range selectedResources(p, opts) {
			if len(resource.Attributes) == 0 {
				line, err := marshalJSONLine(map[string]any{
					"a":     string(resource.Action),
					"addr":  resource.Address,
					"attrs": "none",
					"flags": resource.SummaryFlags(),
				})
				if err != nil {
					return "", err
				}
				lines = append(lines, line)
				continue
			}
			for _, attribute := range resource.Attributes {
				line, err := marshalJSONLine(map[string]any{
					"a":     string(resource.Action),
					"addr":  resource.Address,
					"p":     attribute.Path,
					"b":     jsonValue(attribute.Before, opts.Limits),
					"n":     jsonValue(attribute.After, opts.Limits),
					"flags": attribute.Flags,
				})
				if err != nil {
					return "", err
				}
				lines = append(lines, line)
			}
		}
		if !opts.EssentialOnly && !opts.RiskOnly {
			for _, output := range p.Outputs {
				for _, attribute := range output.Attributes {
					line, err := marshalJSONLine(map[string]any{
						"a":     string(plan.ActionOutput),
						"addr":  output.Address,
						"p":     attribute.Path,
						"b":     jsonValue(attribute.Before, opts.Limits),
						"n":     jsonValue(attribute.After, opts.Limits),
						"flags": attribute.Flags,
					})
					if err != nil {
						return "", err
					}
					lines = append(lines, line)
				}
			}
		}
	}
	if opts.Omitted > 0 {
		line, err := marshalJSONLine(summaryObject(p.Summary, opts.Omitted))
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// RenderMarkdown emits a concise review-friendly table.
func RenderMarkdown(p *plan.Plan, opts Options) string {
	var b strings.Builder
	b.WriteString("### Terraform Plan Context\n\n")
	b.WriteByte('`')
	b.WriteString(headerLine(p.Summary, opts.Omitted))
	b.WriteString("`\n")
	if opts.HeaderOnly {
		return b.String()
	}
	if opts.Summary {
		b.WriteString("\n| Action | Address | Changes | Flags |\n")
		b.WriteString("| --- | --- | ---: | --- |\n")
		for _, resource := range selectedResources(p, opts) {
			fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", resource.Action, markdownCell(resource.Address), len(resource.Attributes), markdownCell(strings.Join(resource.SummaryFlags(), ",")))
		}
		if !opts.EssentialOnly && !opts.RiskOnly {
			for _, output := range p.Outputs {
				fmt.Fprintf(&b, "| %s | %s | %d |  |\n", plan.ActionOutput, markdownCell(output.Address), len(output.Attributes))
			}
		}
		if opts.IncludeNoOp {
			for _, resource := range p.NoOpResources {
				fmt.Fprintf(&b, "| %s | %s | 0 |  |\n", resource.Action, markdownCell(resource.Address))
			}
		}
		return b.String()
	}

	b.WriteString("\n| Action | Address | Path | Before | After | Flags |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, resource := range selectedResources(p, opts) {
		if len(resource.Attributes) == 0 {
			fmt.Fprintf(&b, "| %s | %s | self | null | null | %s |\n",
				resource.Action,
				markdownCell(resource.Address),
				markdownCell(strings.Join(resource.SummaryFlags(), ",")),
			)
			continue
		}
		for _, attribute := range resource.Attributes {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
				resource.Action,
				markdownCell(resource.Address),
				markdownCell(attribute.Path),
				markdownCell(lineValue(attribute.Before, opts.Limits)),
				markdownCell(lineValue(attribute.After, opts.Limits)),
				markdownCell(strings.Join(attribute.Flags, ",")),
			)
		}
	}
	if !opts.EssentialOnly && !opts.RiskOnly {
		for _, output := range p.Outputs {
			for _, attribute := range output.Attributes {
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
					plan.ActionOutput,
					markdownCell(output.Address),
					markdownCell(attribute.Path),
					markdownCell(lineValue(attribute.Before, opts.Limits)),
					markdownCell(lineValue(attribute.After, opts.Limits)),
					markdownCell(strings.Join(attribute.Flags, ",")),
				)
			}
		}
	}
	return b.String()
}

func selectedResources(p *plan.Plan, opts Options) []plan.ResourceChange {
	resources := make([]plan.ResourceChange, 0, len(p.Resources))
	for _, resource := range p.Resources {
		if opts.RiskOnly && len(resource.Risks) == 0 {
			continue
		}
		if opts.EssentialOnly && len(resource.Risks) == 0 && resource.Action != plan.ActionReplace && resource.Action != plan.ActionDelete && resource.Action != plan.ActionCreate {
			continue
		}
		resources = append(resources, resource)
	}
	return resources
}

func headerLine(summary plan.PlanSummary, omitted int) string {
	header := fmt.Sprintf("TFP1 C=%d U=%d R=%d D=%d Q=%d OUT=%d RISK=%d", summary.Creates, summary.Updates, summary.Replaces, summary.Deletes, summary.Reads, summary.OutputChanges, summary.RiskResources)
	if omitted > 0 {
		header += fmt.Sprintf(" OMITTED=%d", omitted)
	}
	return header
}

func summaryObject(summary plan.PlanSummary, omitted int) map[string]any {
	object := map[string]any{
		"a":    "H",
		"c":    summary.Creates,
		"u":    summary.Updates,
		"r":    summary.Replaces,
		"d":    summary.Deletes,
		"q":    summary.Reads,
		"out":  summary.OutputChanges,
		"risk": summary.RiskResources,
	}
	if omitted > 0 {
		object["omitted"] = omitted
	}
	return object
}

func lineValue(value plan.Value, limits Limits) string {
	rendered, _ := lineValueWithSummary(value, limits)
	return rendered
}

func lineValueWithSummary(value plan.Value, limits Limits) (string, bool) {
	switch value.Kind {
	case plan.ValueNull:
		return "null", false
	case plan.ValueUnknown:
		return "unknown", false
	case plan.ValueSensitive:
		return "sensitive", false
	case plan.ValueExists:
		return "exists", false
	case plan.ValueRaw:
		return compactValueWithSummary(value.Raw, limits)
	default:
		return "null", false
	}
}

func jsonValue(value plan.Value, limits Limits) any {
	switch value.Kind {
	case plan.ValueNull:
		return nil
	case plan.ValueUnknown:
		return "unknown"
	case plan.ValueSensitive:
		return "sensitive"
	case plan.ValueExists:
		return "exists"
	case plan.ValueRaw:
		return compactJSONValue(value.Raw, limits)
	default:
		return nil
	}
}

func compactValue(value any, limits Limits) string {
	rendered, _ := compactValueWithSummary(value, limits)
	return rendered
}

func compactValueWithSummary(value any, limits Limits) (string, bool) {
	if summarized, ok := summaryValue(value, limits); ok {
		return summarized, true
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("json(len=0,sha256=%s)", hashPrefix(nil)), true
	}
	if limits.MaxValueLen > 0 && len(encoded) > limits.MaxValueLen {
		return summarizedEncodedValue(value, encoded), true
	}
	return string(encoded), false
}

func compactJSONValue(value any, limits Limits) any {
	if summarized, ok := summaryValue(value, limits); ok {
		return summarized
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("json(len=0,sha256=%s)", hashPrefix(nil))
	}
	if limits.MaxValueLen > 0 && len(encoded) > limits.MaxValueLen {
		return summarizedEncodedValue(value, encoded)
	}
	return value
}

func summaryValue(value any, limits Limits) (string, bool) {
	switch typed := value.(type) {
	case string:
		if limits.MaxValueLen > 0 && len(typed) > limits.MaxValueLen {
			return fmt.Sprintf("long_string(len=%d,sha256=%s)", len(typed), hashPrefix([]byte(typed))), true
		}
	case []any:
		if limits.MaxListItems > 0 && len(typed) > limits.MaxListItems {
			encoded, _ := json.Marshal(typed)
			return fmt.Sprintf("list(len=%d,sha256=%s)", len(typed), hashPrefix(encoded)), true
		}
	case map[string]any:
		if limits.MaxObjectKeys > 0 && len(typed) > limits.MaxObjectKeys {
			encoded, _ := json.Marshal(typed)
			return summarizedObjectValue(typed, encoded), true
		}
	}
	return "", false
}

func summarizedEncodedValue(value any, encoded []byte) string {
	switch typed := value.(type) {
	case map[string]any:
		return summarizedObjectValue(typed, encoded)
	case []any:
		return fmt.Sprintf("list(len=%d,sha256=%s)", len(typed), hashPrefix(encoded))
	default:
		return fmt.Sprintf("json(len=%d,sha256=%s)", len(encoded), hashPrefix(encoded))
	}
}

func summarizedObjectValue(value map[string]any, encoded []byte) string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 3 {
		keys = keys[:3]
	}
	return fmt.Sprintf("object(len=%d,sha256=%s,keys=%s)", len(encoded), hashPrefix(encoded), strings.Join(keys, ","))
}

func hashPrefix(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])[:12]
}

func markdownCell(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func marshalJSONLine(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// SortedFlags returns a copy of flags in deterministic order. It is useful in tests and future callers.
func SortedFlags(flags []string) []string {
	copied := append([]string(nil), flags...)
	sort.Strings(copied)
	return copied
}
