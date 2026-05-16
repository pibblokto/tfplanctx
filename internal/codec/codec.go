package codec

import (
	"fmt"
	"strconv"
	"strings"
)

// Escape encodes compact-protocol delimiters while keeping ordinary Terraform paths readable.
func Escape(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '-' && i+1 < len(value) && value[i+1] == '>' {
			b.WriteString("%2D%3E")
			i++
			continue
		}
		switch value[i] {
		case '%', '|', ';', '=', '\n', '\r':
			fmt.Fprintf(&b, "%%%02X", value[i])
		default:
			b.WriteByte(value[i])
		}
	}
	return b.String()
}

// EscapeListItem additionally protects comma-separated metadata lists.
func EscapeListItem(value string) string {
	return strings.ReplaceAll(Escape(value), ",", "%2C")
}

// Unescape reverses Escape.
func Unescape(value string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			b.WriteByte(value[i])
			continue
		}
		if i+2 >= len(value) {
			return "", fmt.Errorf("invalid escape at byte %d", i)
		}
		decoded, err := strconv.ParseUint(value[i+1:i+3], 16, 8)
		if err != nil {
			return "", fmt.Errorf("invalid escape %q: %w", value[i:i+3], err)
		}
		b.WriteByte(byte(decoded))
		i += 2
	}
	return b.String(), nil
}
