package server

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// Create a temporary directory with test emote files to verify loading logic.
	tmpDir := t.TempDir()

	// Create minimal test files (empty files are sufficient for this test)
	testFiles := []string{"wave.svg", "clap.png", "fire.gif"}
	for _, f := range testFiles {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte{}, 0644); err != nil {
			t.Fatalf("failed to create test emote file %q: %v", f, err)
		}
	}

	// Scan the temp directory
	scanImageEmotes(tmpDir)

	// Verify image emotes were loaded
	expectedImages := []string{"wave", "clap", "fire"}
	for _, name := range expectedImages {
		replacement, ok := emoteMap[name]
		if !ok {
			t.Errorf("image emote %q not found in emoteMap", name)
			continue
		}
		// Check it's an img tag with the correct source path
		if !strings.Contains(replacement, `<img src="/emotes/`) {
			t.Errorf("image emote %q is not a runtime img tag: %q", name, replacement)
		}
		if !strings.Contains(replacement, `class="emote"`) {
			t.Errorf("image emote %q missing emote class", name)
		}
	}

	// Verify text emoji are still present
	for name := range textEmoteNames {
		if _, ok := emoteMap[name]; !ok {
			t.Errorf("text emote %q was removed from emoteMap", name)
		}
	}
}

func TestResolveEmotePath(t *testing.T) {
	// Save and restore original env vars
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_DATA_HOME")
	origGrunt := os.Getenv("GRUNT_EMOTE_DIR")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("XDG_DATA_HOME", origXDG)
		os.Setenv("GRUNT_EMOTE_DIR", origGrunt)
	}()

	tests := []struct {
		name     string
		home     string
		xdg      string
		gruntDir string
		expected string
	}{
		{
			name:     "GRUNT_EMOTE_DIR override",
			gruntDir: "/custom/emotes",
			expected: "/custom/emotes",
		},
		{
			name:   "XDG_DATA_HOME set",
			xdg:    "/tmp/xdg",
			home:   "/tmp/home",
			expected: "/tmp/xdg/grunt/emotes",
		},
		{
			name:   "HOME fallback",
			home:   "/tmp/home",
			expected: "/tmp/home/.local/share/grunt/emotes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("GRUNT_EMOTE_DIR", tt.gruntDir)
			os.Setenv("XDG_DATA_HOME", tt.xdg)
			os.Setenv("HOME", tt.home)

			got := resolveEmotePath()
			if got != tt.expected {
				t.Errorf("resolveEmotePath() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestEmoteMapRaceDuringReload guards against a data race between the
// fsnotify-triggered reload path (scanImageEmotes) and concurrent message
// rendering (ReplaceEmotes). It creates/removes emote files in a tight loop
// while a reader goroutine renders messages that reference the emote, forcing
// overlapping map reads and writes. Run with -race; it fails if any map
// mutation in scanImageEmotes happens outside emoteMu.
func TestEmoteMapRaceDuringReload(t *testing.T) {
	tmpDir := t.TempDir()
	const emoteName = "raceemote"
	filePath := filepath.Join(tmpDir, emoteName+".png")

	// Keep the reader continuously active for the whole reload window so the
	// two goroutines are guaranteed to overlap in time.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = ReplaceEmotes("hello :" + emoteName + ": world")
			}
		}
	}()

	// Simulate the watcher's reload: repeatedly add and remove the emote file,
	// rescanning after each change so the global emoteMap is mutated under the
	// reader's concurrent access.
	for i := 0; i < 200; i++ {
		if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
			t.Fatalf("failed to create emote file: %v", err)
		}
		scanImageEmotes(tmpDir)
		if err := os.Remove(filePath); err != nil {
			t.Fatalf("failed to remove emote file: %v", err)
		}
		scanImageEmotes(tmpDir)
	}

	close(stop)
	wg.Wait()

	// Drop the test emote from the global map.
	scanImageEmotes(t.TempDir())
}
