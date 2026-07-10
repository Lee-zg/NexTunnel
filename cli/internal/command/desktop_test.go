package command

import "testing"

func TestNormalizeDesktopPublishAuthMode(t *testing.T) {
	tests := map[string]string{
		"":             "none",
		"none":         "none",
		"basic":        "basic_auth",
		"BASIC_AUTH":   "basic_auth",
		"bearer":       "bearer_token",
		"bearer_token": "bearer_token",
	}
	for input, want := range tests {
		got, err := normalizeDesktopPublishAuthMode(input)
		if err != nil {
			t.Fatalf("normalizeDesktopPublishAuthMode(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeDesktopPublishAuthMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeDesktopPublishAuthModeRejectsUnknownValue(t *testing.T) {
	if _, err := normalizeDesktopPublishAuthMode("typo"); err == nil {
		t.Fatal("expected unsupported auth mode error")
	}
}
