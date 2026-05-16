package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsCompressionTable(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, filepath.Join("..", "..", "testdata")); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"fixture",
		"raw_tokens",
		"tfp1_tokens",
		"tokens_saved",
		"reduction",
		"plan_main.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("table output missing %q: %s", want, got)
		}
	}
}
