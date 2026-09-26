package cli

import (
	"bufio"
	"strings"
	"testing"
)

func TestIntOrDefault(t *testing.T) {
	tests := []struct {
		val      int
		fallback string
		want     string
	}{
		{0, "default", "default"},
		{0, "", ""},
		{5, "default", "5"},
		{100, "", "100"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := intOrDefault(tt.val, tt.fallback)
			if got != tt.want {
				t.Errorf("intOrDefault(%d, %q) = %q, want %q", tt.val, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestBoolStr(t *testing.T) {
	tests := []struct {
		val  bool
		want string
	}{
		{true, "true"},
		{false, "false"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := boolStr(tt.val)
			if got != tt.want {
				t.Errorf("boolStr(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestPrompt_WithInput(t *testing.T) {
	input := strings.NewReader("user input\n")
	reader := bufio.NewReader(input)

	got := prompt(reader, "Label", "default")
	if got != "user input" {
		t.Errorf("prompt() = %q, want %q", got, "user input")
	}
}

func TestPrompt_WithDefault(t *testing.T) {
	input := strings.NewReader("\n")
	reader := bufio.NewReader(input)

	got := prompt(reader, "Label", "default")
	if got != "default" {
		t.Errorf("prompt() = %q, want %q", got, "default")
	}
}
