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
		"json_tokens",
		"review_tokens",
		"detail_tokens",
		"review_saved",
		"review_reduction",
		"plan_main.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("table output missing %q: %s", want, got)
		}
	}
}
