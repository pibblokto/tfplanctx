package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/pibblokto/tfplanctx/internal/benchmark"
	"github.com/pibblokto/tfplanctx/internal/plan"
	"github.com/pibblokto/tfplanctx/internal/render"
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
		review, err := render.Render("compact", parsed, render.Options{Limits: render.DefaultLimits()})
		if err != nil {
			return fmt.Errorf("render %s: %w", path, err)
		}
		detail, err := render.Render("compact", parsed, render.Options{Detail: true, Limits: render.DefaultLimits()})
		if err != nil {
			return fmt.Errorf("render detail %s: %w", path, err)
		}
		row := benchmark.Row{
			Name:   filepath.Base(path),
			Review: benchmark.Compare(data, review),
			Detail: benchmark.Compare(data, detail),
		}
		rows = append(rows, row)
	}

	return benchmark.WriteTable(stdout, rows)
}
