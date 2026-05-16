package input

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Load returns Terraform JSON plan content from a JSON file, stdin, or binary plan file.
func Load(ctx context.Context, path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read JSON plan from stdin: %w", err)
		}
		if !json.Valid(data) {
			return nil, fmt.Errorf("stdin does not contain valid Terraform JSON plan content")
		}
		return data, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read input %q: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("input %q is empty", path)
	}

	trimmed := bytes.TrimSpace(data)
	if trimmed[0] == '{' || trimmed[0] == '[' {
		if !json.Valid(data) {
			return nil, fmt.Errorf("input %q appears to be JSON but is not valid", path)
		}
		return data, nil
	}
	if looksLikeHumanPlan(trimmed) {
		return nil, fmt.Errorf("input %q appears to be human-readable Terraform plan output; only binary plan files and Terraform JSON are supported", path)
	}

	cmd := exec.CommandContext(ctx, "terraform", "show", "-json", path)
	stdout, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("run terraform show -json %q: %w: %s", path, err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("run terraform show -json %q: %w", path, err)
	}
	if !json.Valid(stdout) {
		return nil, fmt.Errorf("terraform show -json %q returned invalid JSON", path)
	}
	return stdout, nil
}

func looksLikeHumanPlan(data []byte) bool {
	return bytes.HasPrefix(data, []byte("Terraform will perform the following actions:")) ||
		bytes.HasPrefix(data, []byte("No changes. Your infrastructure matches the configuration."))
}
