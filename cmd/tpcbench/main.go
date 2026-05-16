package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/piblokto/tfplanctx/internal/benchmark"
	"github.com/piblokto/tfplanctx/internal/plan"
	"github.com/piblokto/tfplanctx/internal/render"
)

func main() {
	if err := run(os.Stdout, "testdata"); err != nil {
		fmt.Fprintf(os.Stderr, "tpcbench: %v\n", err)
		os.Exit(1)
	}
}

func run(stdout io.Writer, fixtureDir string) error {
	paths, err := filepath.Glob(filepath.Join(fixtureDir, "plan_*.json"))
	if err != nil {
		return fmt.Errorf("find benchmark fixtures: %w", err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("no benchmark fixtures found")
	}
	sort.Strings(paths)

	rows := make([]benchmark.Row, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		parsed, err := plan.Parse(data, plan.ParseOptions{})
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		output, err := render.Render("line", parsed, render.Options{Limits: render.DefaultLimits()})
		if err != nil {
			return fmt.Errorf("render %s: %w", path, err)
		}
		rows = append(rows, benchmark.Row{
			Name:   filepath.Base(path),
			Report: benchmark.Compare(data, output),
		})
	}

	return benchmark.WriteTable(stdout, rows)
}
