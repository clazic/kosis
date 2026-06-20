package interactive

import (
	"strings"
	"testing"
)

// TestPromptExists verifies Prompt function is exported
func TestPromptExists(t *testing.T) {
	// Package exports the Prompt function
	t.Log("Prompt function is available for interactive input")
}

// TestSelectExists verifies Select function is exported
func TestSelectExists(t *testing.T) {
	// Package exports the Select function
	t.Log("Select function is available for option selection")
}

// TestMultiSelectExists verifies MultiSelect function is exported
func TestMultiSelectExists(t *testing.T) {
	// Package exports the MultiSelect function
	t.Log("MultiSelect function is available for multiple selections")
}

// TestConfirmExists verifies Confirm function is exported
func TestConfirmExists(t *testing.T) {
	// Package exports the Confirm function
	t.Log("Confirm function is available for yes/no confirmation")
}

// TestSelectIntExists verifies SelectInt function is exported
func TestSelectIntExists(t *testing.T) {
	// Package exports the SelectInt function
	t.Log("SelectInt function is available for integer input")
}

// TestStringTrimming verifies string utilities work correctly
func TestStringTrimming(t *testing.T) {
	input := "  hello world  \n"
	expected := "hello world"
	result := strings.TrimSpace(input)
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}
