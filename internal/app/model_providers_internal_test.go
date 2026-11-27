package app

import "testing"

// L1 (i119 review): a short key is not kept, or shown, half in the clear.
func TestKeyHintShowsNothingOfAShortKey(t *testing.T) {
	for key, want := range map[string]string{
		"": "", "abcd": "", "12345678": "", "sk-12345678": "", "sk-123456789": "6789", "mm-hub-key-0123456789abcd": "abcd",
	} {
		if got := KeyHint(key); got != want {
			t.Errorf("KeyHint(%q) = %q, want %q", key, got, want)
		}
	}
}
