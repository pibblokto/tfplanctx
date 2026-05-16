package codec

import "testing"

func TestEscapeRoundTrip(t *testing.T) {
	input := "labels[\"a.b[0]|x\"]=old->new;line\nnext,more%"
	encoded := Escape(input)
	if encoded == input {
		t.Fatal("expected delimiters to be escaped")
	}
	decoded, err := Unescape(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != input {
		t.Fatalf("decoded = %q, want %q", decoded, input)
	}
}
