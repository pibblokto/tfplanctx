package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/piblokto/tfplanctx/internal/budget"
	"github.com/piblokto/tfplanctx/internal/input"
	"github.com/piblokto/tfplanctx/internal/plan"
	"github.com/piblokto/tfplanctx/internal/redact"
	"github.com/piblokto/tfplanctx/internal/render"
)

type config struct {
	format                        string
	summary                       bool
	riskOnly                      bool
	resource                      string
	resourceType                  string
	budget                        int
	includeRead                   bool
	includeNoOp                   bool
	noColor                       bool
	detailedExitCode              bool
	unsafeShowSensitive           bool
	unsafeDisableSecretHeuristics bool
	limits                        render.Limits
	inputPath                     string
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	content, err := input.Load(ctx, cfg.inputPath, os.Stdin)
	if err != nil {
		fatal(err)
	}

	normalized, err := plan.Parse(content, plan.ParseOptions{
		IncludeRead: cfg.includeRead,
		Redact: redact.Config{
			UnsafeShowSensitive:           cfg.unsafeShowSensitive,
			UnsafeDisableSecretHeuristics: cfg.unsafeDisableSecretHeuristics,
		},
	})
	if err != nil {
		fatal(err)
	}

	view := normalized.Filter(cfg.resource, cfg.resourceType)
	opts := render.Options{
		Summary:     cfg.summary,
		RiskOnly:    cfg.riskOnly,
		IncludeNoOp: cfg.includeNoOp,
		Limits:      cfg.limits,
	}

	var output string
	if cfg.budget > 0 {
		output, _, err = budget.Fit(view, cfg.format, opts, cfg.budget)
	} else {
		output, err = render.Render(cfg.format, view, opts)
	}
	if err != nil {
		fatal(err)
	}
	fmt.Print(output)

	if cfg.detailedExitCode {
		switch {
		case normalized.HasRisks():
			os.Exit(3)
		case normalized.HasChanges():
			os.Exit(2)
		default:
			os.Exit(0)
		}
	}
}

func parseArgs(args []string) (config, error) {
	var cfg config
	cfg.limits = render.DefaultLimits()

	fs := flag.NewFlagSet("tpc", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.format, "format", "line", "output format: line, jsonl, or markdown")
	fs.BoolVar(&cfg.summary, "summary", false, "emit one line per changed resource")
	fs.BoolVar(&cfg.riskOnly, "risk-only", false, "emit only risky changed resources")
	fs.StringVar(&cfg.resource, "resource", "", "emit only the exact Terraform resource address")
	fs.StringVar(&cfg.resourceType, "type", "", "emit only the exact Terraform resource type")
	fs.IntVar(&cfg.budget, "budget", 0, "approximate output character budget")
	fs.BoolVar(&cfg.includeRead, "include-read", false, "include read/data-source style changes")
	fs.BoolVar(&cfg.includeNoOp, "include-noop", false, "include no-op resource addresses in summary mode")
	fs.BoolVar(&cfg.noColor, "no-color", false, "compatibility flag; output never uses color")
	fs.BoolVar(&cfg.detailedExitCode, "detailed-exitcode", false, "use Terraform-like detailed exit codes")
	fs.BoolVar(&cfg.unsafeShowSensitive, "unsafe-show-sensitive", false, "print Terraform-marked sensitive values")
	fs.BoolVar(&cfg.unsafeDisableSecretHeuristics, "unsafe-disable-secret-heuristics", false, "disable heuristic secret-path redaction")
	fs.IntVar(&cfg.limits.MaxValueLen, "max-value-len", cfg.limits.MaxValueLen, "maximum rendered value length before summarization")
	fs.IntVar(&cfg.limits.MaxListItems, "max-list-items", cfg.limits.MaxListItems, "maximum list items before summarization")
	fs.IntVar(&cfg.limits.MaxObjectKeys, "max-object-keys", cfg.limits.MaxObjectKeys, "maximum object keys before summarization")

	normalizedArgs, err := reorderArgs(args)
	if err != nil {
		return cfg, err
	}
	if err := fs.Parse(normalizedArgs); err != nil {
		return cfg, err
	}
	if cfg.format != "line" && cfg.format != "jsonl" && cfg.format != "markdown" {
		return cfg, fmt.Errorf("unsupported format %q", cfg.format)
	}
	if cfg.budget < 0 {
		return cfg, fmt.Errorf("budget must be non-negative")
	}
	if cfg.limits.MaxValueLen < 0 || cfg.limits.MaxListItems < 0 || cfg.limits.MaxObjectKeys < 0 {
		return cfg, fmt.Errorf("value limits must be non-negative")
	}

	positionals := fs.Args()
	if len(positionals) != 1 {
		return cfg, fmt.Errorf("expected exactly one input path or '-' for stdin")
	}
	cfg.inputPath = positionals[0]
	_ = cfg.noColor
	return cfg, nil
}

func reorderArgs(args []string) ([]string, error) {
	valueFlags := map[string]struct{}{
		"--format":          {},
		"--resource":        {},
		"--type":            {},
		"--budget":          {},
		"--max-value-len":   {},
		"--max-list-items":  {},
		"--max-object-keys": {},
	}

	var flags []string
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}
		flags = append(flags, arg)
		name := arg
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			name = arg[:idx]
		}
		if _, ok := valueFlags[name]; ok && !strings.Contains(arg, "=") {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("flag %s requires a value", arg)
			}
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positionals...), nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "tpc: %v\n", err)
	os.Exit(1)
}
