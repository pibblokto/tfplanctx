package redact

import (
	"strings"

	"github.com/pibblokto/tfplanctx/internal/diff"
)

var heuristicTerms = []string{
	"password",
	"passwd",
	"secret",
	"token",
	"api_key",
	"apikey",
	"private_key",
	"client_secret",
	"authorization",
	"cookie",
	"session",
	"credential",
}

// Config controls explicit unsafe redaction overrides.
type Config struct {
	UnsafeShowSensitive           bool
	UnsafeDisableSecretHeuristics bool
}

// ShouldRedact reports whether the value at path must be replaced with sensitive.
func ShouldRedact(path diff.Path, beforeSensitive, afterSensitive any, cfg Config) bool {
	if !cfg.UnsafeDisableSecretHeuristics && HasHeuristicSecret(path) {
		return true
	}
	if cfg.UnsafeShowSensitive {
		return false
	}
	return IsSensitive(beforeSensitive, path) || IsSensitive(afterSensitive, path)
}

// HasHeuristicSecret applies conservative path-name redaction.
func HasHeuristicSecret(path diff.Path) bool {
	for _, segment := range path {
		if segment.IsIndex {
			continue
		}
		key := strings.ToLower(segment.Key)
		for _, term := range heuristicTerms {
			if strings.Contains(key, term) {
				return true
			}
		}
	}
	return false
}

// IsSensitive follows Terraform's nested before_sensitive/after_sensitive trees.
func IsSensitive(meta any, path diff.Path) bool {
	current := meta
	if sensitive, ok := current.(bool); ok {
		return sensitive
	}

	for _, segment := range path {
		if sensitive, ok := current.(bool); ok {
			return sensitive
		}

		if segment.IsIndex {
			items, ok := current.([]any)
			if !ok || segment.Index < 0 || segment.Index >= len(items) {
				return false
			}
			current = items[segment.Index]
			continue
		}

		mapped, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current = mapped[segment.Key]
	}

	if sensitive, ok := current.(bool); ok {
		return sensitive
	}
	return false
}
