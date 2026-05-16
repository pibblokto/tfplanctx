package diff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var simpleKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Segment is one path component within a Terraform value.
type Segment struct {
	Key     string
	Index   int
	IsIndex bool
}

// Path identifies a nested value using Terraform-like dot/index notation.
type Path []Segment

// WithKey returns a copy of the path with an object key appended.
func (p Path) WithKey(key string) Path {
	next := append(Path(nil), p...)
	return append(next, Segment{Key: key})
}

// WithIndex returns a copy of the path with a list index appended.
func (p Path) WithIndex(index int) Path {
	next := append(Path(nil), p...)
	return append(next, Segment{Index: index, IsIndex: true})
}

// String returns a deterministic, grep-friendly representation of the path.
func (p Path) String() string {
	if len(p) == 0 {
		return ""
	}

	var b strings.Builder
	for i, segment := range p {
		if segment.IsIndex {
			fmt.Fprintf(&b, "[%d]", segment.Index)
			continue
		}

		if simpleKeyPattern.MatchString(segment.Key) {
			if i > 0 {
				b.WriteByte('.')
			}
			b.WriteString(segment.Key)
			continue
		}

		encoded, _ := json.Marshal(segment.Key)
		b.WriteByte('[')
		b.Write(encoded)
		b.WriteByte(']')
	}
	return b.String()
}

// HasPrefix reports whether prefix is a leading path of p.
func (p Path) HasPrefix(prefix Path) bool {
	if len(prefix) > len(p) {
		return false
	}
	for i := range prefix {
		if p[i] != prefix[i] {
			return false
		}
	}
	return true
}

// Change is one changed leaf value.
type Change struct {
	Path         Path
	Before       any
	After        any
	AfterUnknown bool
}

// Changes recursively compares before and after, returning changed leaves only.
func Changes(before, after, afterUnknown any) []Change {
	var out []Change
	walk(nil, before, after, afterUnknown, &out)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path.String() < out[j].Path.String()
	})
	return out
}

// FromRawPath converts Terraform replace_paths segments into a normalized path.
func FromRawPath(raw []any) Path {
	var path Path
	for _, segment := range raw {
		switch value := segment.(type) {
		case string:
			path = path.WithKey(value)
		case json.Number:
			if n, err := value.Int64(); err == nil {
				path = path.WithIndex(int(n))
			}
		case float64:
			path = path.WithIndex(int(value))
		case float32:
			path = path.WithIndex(int(value))
		case int:
			path = path.WithIndex(value)
		case int64:
			path = path.WithIndex(int(value))
		}
	}
	return path
}

func walk(path Path, before, after, afterUnknown any, out *[]Change) {
	if isUnknown(afterUnknown) {
		*out = append(*out, Change{Path: append(Path(nil), path...), Before: before, After: after, AfterUnknown: true})
		return
	}

	if reflect.DeepEqual(before, after) && !hasUnknown(afterUnknown) {
		return
	}

	if beforeMap, ok := asMap(before); ok {
		if afterMap, ok := asMap(after); ok {
			for _, key := range unionMapKeys(beforeMap, afterMap) {
				walk(path.WithKey(key), beforeMap[key], afterMap[key], childUnknown(afterUnknown, key), out)
			}
			return
		}
		if after == nil {
			keys := sortedMapKeys(beforeMap)
			for _, key := range keys {
				walk(path.WithKey(key), beforeMap[key], nil, childUnknown(afterUnknown, key), out)
			}
			return
		}
	}

	if afterMap, ok := asMap(after); ok && before == nil {
		keys := sortedMapKeys(afterMap)
		for _, key := range keys {
			walk(path.WithKey(key), nil, afterMap[key], childUnknown(afterUnknown, key), out)
		}
		return
	}

	if beforeList, ok := asSlice(before); ok {
		if afterList, ok := asSlice(after); ok {
			if isLeafList(beforeList) && isLeafList(afterList) {
				*out = append(*out, Change{Path: append(Path(nil), path...), Before: before, After: after})
				return
			}
			maxLen := max(len(beforeList), len(afterList))
			for i := 0; i < maxLen; i++ {
				var beforeValue, afterValue any
				if i < len(beforeList) {
					beforeValue = beforeList[i]
				}
				if i < len(afterList) {
					afterValue = afterList[i]
				}
				walk(path.WithIndex(i), beforeValue, afterValue, childUnknown(afterUnknown, i), out)
			}
			return
		}
		if after == nil {
			if isLeafList(beforeList) {
				*out = append(*out, Change{Path: append(Path(nil), path...), Before: before, After: nil})
				return
			}
			for i, value := range beforeList {
				walk(path.WithIndex(i), value, nil, childUnknown(afterUnknown, i), out)
			}
			return
		}
	}

	if afterList, ok := asSlice(after); ok && before == nil {
		if isLeafList(afterList) {
			*out = append(*out, Change{Path: append(Path(nil), path...), Before: nil, After: after})
			return
		}
		for i, value := range afterList {
			walk(path.WithIndex(i), nil, value, childUnknown(afterUnknown, i), out)
		}
		return
	}

	*out = append(*out, Change{Path: append(Path(nil), path...), Before: before, After: after})
}

func hasUnknown(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case map[string]any:
		for _, child := range typed {
			if hasUnknown(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasUnknown(child) {
				return true
			}
		}
	}
	return false
}

func isUnknown(value any) bool {
	unknown, ok := value.(bool)
	return ok && unknown
}

func childUnknown(value any, key any) any {
	switch typed := value.(type) {
	case map[string]any:
		if stringKey, ok := key.(string); ok {
			return typed[stringKey]
		}
	case []any:
		if index, ok := key.(int); ok && index >= 0 && index < len(typed) {
			return typed[index]
		}
	}
	return nil
}

func asMap(value any) (map[string]any, bool) {
	mapped, ok := value.(map[string]any)
	return mapped, ok
}

func asSlice(value any) ([]any, bool) {
	sliced, ok := value.([]any)
	return sliced, ok
}

func isLeafList(values []any) bool {
	for _, value := range values {
		switch value.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

func unionMapKeys(left, right map[string]any) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		seen[key] = struct{}{}
	}
	for key := range right {
		seen[key] = struct{}{}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
