package diff

import "testing"

func TestScalarListsStayAsSingleLeaf(t *testing.T) {
	changes := Changes(
		map[string]any{"cidr_blocks": []any{"10.0.0.0/8"}},
		map[string]any{"cidr_blocks": []any{"0.0.0.0/0"}},
		nil,
	)
	if got, want := len(changes), 1; got != want {
		t.Fatalf("change count = %d, want %d", got, want)
	}
	if got, want := changes[0].Path.String(), "cidr_blocks"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestObjectListsStillRecurse(t *testing.T) {
	changes := Changes(
		map[string]any{"rules": []any{map[string]any{"port": 80}}},
		map[string]any{"rules": []any{map[string]any{"port": 443}}},
		nil,
	)
	if got, want := len(changes), 1; got != want {
		t.Fatalf("change count = %d, want %d", got, want)
	}
	if got, want := changes[0].Path.String(), "rules[0].port"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestAfterUnknownCreatesChangeEvenWhenValuesMatch(t *testing.T) {
	changes := Changes(
		map[string]any{"id": "i-123"},
		map[string]any{"id": "i-123"},
		map[string]any{"id": true},
	)
	if got, want := len(changes), 1; got != want {
		t.Fatalf("change count = %d, want %d", got, want)
	}
	if !changes[0].AfterUnknown {
		t.Fatalf("expected unknown marker, got %#v", changes[0])
	}
}

func TestSpecialMapKeysRenderDeterministically(t *testing.T) {
	changes := Changes(
		map[string]any{"labels": map[string]any{"app.kubernetes.io/name": "old"}},
		map[string]any{"labels": map[string]any{"app.kubernetes.io/name": "new"}},
		nil,
	)
	if got, want := changes[0].Path.String(), `labels["app.kubernetes.io/name"]`; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
