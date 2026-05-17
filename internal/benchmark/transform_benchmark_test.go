package benchmark_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pibblokto/tfplanctx/internal/budget"
	"github.com/pibblokto/tfplanctx/internal/plan"
	"github.com/pibblokto/tfplanctx/internal/render"
)

func BenchmarkTransformCompact(b *testing.B) {
	data := mustReadFixture(b, "plan_main.json")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parsed, err := plan.Parse(data, plan.ParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := render.Render("compact", parsed, render.Options{Limits: render.DefaultLimits()}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransformCompactDetail(b *testing.B) {
	data := mustReadFixture(b, "plan_main.json")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parsed, err := plan.Parse(data, plan.ParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := render.Render("compact", parsed, render.Options{Detail: true, Limits: render.DefaultLimits()}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransformCompactSummary(b *testing.B) {
	data := mustReadFixture(b, "plan_main.json")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parsed, err := plan.Parse(data, plan.ParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := render.Render("compact", parsed, render.Options{Summary: true, Limits: render.DefaultLimits()}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransformBudgeted(b *testing.B) {
	data := mustReadFixture(b, "plan_main.json")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parsed, err := plan.Parse(data, plan.ParseOptions{})
		if err != nil {
			b.Fatal(err)
		}
		if _, _, err := budget.Fit(parsed, "compact", render.Options{Limits: render.DefaultLimits()}, 4000); err != nil {
			b.Fatal(err)
		}
	}
}

func mustReadFixture(tb testing.TB, name string) []byte {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		tb.Fatal(err)
	}
	return data
}
