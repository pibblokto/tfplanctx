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
	if !strings.HasPrefix(stdout.String(), "TFP1 ") {
		t.Fatalf("stdout missing plan output: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "BENCH ") {
		t.Fatalf("benchmark polluted stdout: %s", stdout.String())
	}
	bench := stderr.String()
	for _, part := range []string{"BENCH ", "approx_tokens_in=", "approx_tokens_out=", "tokens_saved=", "reduction="} {
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
