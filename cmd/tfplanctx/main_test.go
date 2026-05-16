package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBenchmarkWritesToStderrWithoutChangingStdout(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "plan_main.json")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{fixture, "--summary", "--benchmark"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "TFP2 ") {
		t.Fatalf("stdout missing plan output: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "BENCH ") {
		t.Fatalf("benchmark polluted stdout: %s", stdout.String())
	}
	bench := stderr.String()
	for _, part := range []string{"BENCH ", "json_tokens=", "review_tokens=", "detail_tokens=", "review_vs_json_reduction=", "grouped_resources="} {
		if !strings.Contains(bench, part) {
			t.Fatalf("benchmark output missing %q: %s", part, bench)
		}
	}
}

func TestRunBenchmarkCanCompareAgainstTextPlan(t *testing.T) {
	jsonFixture := filepath.Join("..", "..", "testdata", "plan_main.json")
	textFixture := filepath.Join("..", "..", "testdata", "plan_main.tfplan.txt")
	var stdout, stderr bytes.Buffer
	code := run(
		context.Background(),
		[]string{jsonFixture, "--benchmark", "--txt-plan", textFixture},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
	bench := stderr.String()
	for _, part := range []string{"txt_tokens=", "txt_chars=", "review_vs_txt_reduction=", "detail_vs_txt_reduction="} {
		if !strings.Contains(bench, part) {
			t.Fatalf("benchmark output missing %q: %s", part, bench)
		}
	}
}

func TestRunDetailedExitCodeUsesRiskPrecedence(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "plan_main.json")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{fixture, "--detailed-exitcode"}, strings.NewReader(""), &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3; stderr=%s", code, stderr.String())
	}
}

func TestRunDetailedExitCodeReturnsZeroForNoChanges(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "plan_no_changes.json")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{fixture, "--detailed-exitcode"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
}

func TestParseArgsAcceptsFlagsAfterInput(t *testing.T) {
	cfg, err := parseArgs([]string{"plan.json", "--benchmark", "--format", "jsonl"}, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.benchmark || cfg.format != "jsonl" || cfg.inputPath != "plan.json" {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestParseArgsAcceptsDetail(t *testing.T) {
	cfg, err := parseArgs([]string{"plan.json", "--detail"}, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.detail {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestParseArgsAcceptsSingleDashTxtPlanAfterInput(t *testing.T) {
	cfg, err := parseArgs([]string{"plan.json", "-benchmark", "-txt-plan", "plan.txt"}, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.benchmark || cfg.txtPlanPath != "plan.txt" || cfg.inputPath != "plan.json" {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestParseArgsRejectsTxtPlanWithoutBenchmark(t *testing.T) {
	_, err := parseArgs([]string{"plan.json", "--txt-plan", "plan.txt"}, os.Stderr)
	if err == nil || !strings.Contains(err.Error(), "--txt-plan requires --benchmark") {
		t.Fatalf("err = %v", err)
	}
}
