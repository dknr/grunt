package server

import (
	"html/template"
	"strings"
	"testing"
)

func TestReplaceEmotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected template.HTML
	}{
		{
			name:     "single smile emote",
			input:    ":smile:",
			expected: "😀",
		},
		{
			name:     "single heart emote",
			input:    ":heart:",
			expected: "❤️",
		},
		{
			name:     "multiple emotes",
			input:    ":smile: :heart: :smile:",
			expected: "😀 ❤️ 😀",
		},
		{
			name:     "emote in sentence",
			input:    "hello :smile: world",
			expected: "hello 😀 world",
		},
		{
			name:     "unknown emote left unchanged",
			input:    ":foobar:",
			expected: ":foobar:",
		},
		{
			name:     "mixed known and unknown",
			input:    ":smile: :foobar: :heart:",
			expected: "😀 :foobar: ❤️",
		},
		{
			name:     "no emotes",
			input:    "just plain text",
			expected: "just plain text",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "adjacent emotes",
			input:    ":smile::heart:",
			expected: "😀❤️",
		},
		{
			name:     "emote with surrounding punctuation",
			input:    "(:smile:)",
			expected: "(😀)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceEmotes(tt.input)
			if result != tt.expected {
				t.Errorf("ReplaceEmotes(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEmoteMapCompleteness(t *testing.T) {
	// Ensure all defined emotes are actually replaceable
	for name, replacement := range emoteMap {
		input := ":" + name + ":"
		result := ReplaceEmotes(input)
		if template.HTML(replacement) != result {
			t.Errorf("emote %q not replaced correctly: got %q, want %q", name, result, replacement)
		}
	}
}

func TestImageEmotesLoaded(t *testing.T) {
	// Verify image emotes were loaded during init()
	expectedImages := []string{"wave", "clap", "fire"}
	for _, name := range expectedImages {
		replacement, ok := emoteMap[name]
		if !ok {
			t.Errorf("image emote %q not found in emoteMap", name)
			continue
		}
		// Check it's an img tag
		if !strings.Contains(replacement, `<img src="/static/emotes/`) {
			t.Errorf("image emote %q is not an img tag: %q", name, replacement)
		}
		if !strings.Contains(replacement, `class="emote"`) {
			t.Errorf("image emote %q missing emote class", name)
		}
	}
}