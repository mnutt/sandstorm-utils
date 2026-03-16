package cliutil

import "testing"

func TestNormalizeGrainPath(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"":            "",
		"/":           "",
		"docs/123":    "docs/123",
		"/docs/123":   "docs/123",
		"//docs/123":  "/docs/123",
		"issues/1#x":  "issues/1#x",
		"/issues/1#x": "issues/1#x",
	}

	for input, want := range tests {
		if got := NormalizeGrainPath(input); got != want {
			t.Fatalf("NormalizeGrainPath(%q) = %q, want %q", input, got, want)
		}
	}
}
