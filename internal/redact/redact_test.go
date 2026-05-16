package redact

import (
	"testing"

	"github.com/piblokto/tfplanctx/internal/diff"
)

func TestHasHeuristicSecretMatchesNestedSensitivePath(t *testing.T) {
	path := diff.Path{{Key: "config"}, {Key: "client_secret"}}
	if !HasHeuristicSecret(path) {
		t.Fatal("expected heuristic redaction for client_secret path")
	}
}

func TestHasHeuristicSecretIgnoresSafePath(t *testing.T) {
	path := diff.Path{{Key: "settings"}, {Key: "replicas"}}
	if HasHeuristicSecret(path) {
		t.Fatal("did not expect heuristic redaction for safe path")
	}
}

func TestIsSensitiveFollowsNestedMetadata(t *testing.T) {
	meta := map[string]any{
		"containers": []any{
			map[string]any{"env": map[string]any{"value": true}},
		},
	}
	path := diff.Path{{Key: "containers"}, {Index: 0, IsIndex: true}, {Key: "env"}, {Key: "value"}}
	if !IsSensitive(meta, path) {
		t.Fatal("expected nested metadata path to be sensitive")
	}
}

func TestIsSensitiveReturnsFalseForMissingMetadata(t *testing.T) {
	meta := map[string]any{"password": false}
	path := diff.Path{{Key: "username"}}
	if IsSensitive(meta, path) {
		t.Fatal("did not expect missing metadata path to be sensitive")
	}
}

func TestShouldRedactHonorsUnsafeSensitiveButKeepsHeuristics(t *testing.T) {
	path := diff.Path{{Key: "password"}}
	if !ShouldRedact(path, true, true, Config{UnsafeShowSensitive: true}) {
		t.Fatal("heuristic redaction should remain active even when Terraform-marked sensitive values are allowed")
	}
}

func TestShouldRedactCanDisableHeuristicsExplicitly(t *testing.T) {
	path := diff.Path{{Key: "password"}}
	if ShouldRedact(path, false, false, Config{UnsafeDisableSecretHeuristics: true}) {
		t.Fatal("heuristic redaction should be disabled by explicit unsafe override")
	}
}
