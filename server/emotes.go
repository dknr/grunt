package server

import (
	"html/template"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
)

// emoteMap maps emote names to their display value (emoji character or HTML).
var emoteMap = map[string]string{
	"smile": "😀",
	"heart": "❤️",
}

// emoteRegex matches :name: patterns, allowing hyphens in emote names.
var emoteRegex = regexp.MustCompile(`:[a-zA-Z0-9_-]+:`)

// imageEmoteExtensions lists supported image file extensions for custom emotes.
var imageEmoteExtensions = map[string]bool{
	".svg":  true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

func init() {
	scanImageEmotes()
}

// scanImageEmotes scans the embedded static/emotes directory and adds <img> tags to emoteMap.
// Image emotes take priority over text emoji if there's a name collision.
func scanImageEmotes() {
	entries, err := staticFS.ReadDir("static/emotes")
	if err != nil {
		slog.Warn("No emotes directory found or unable to read it; image emotes disabled", "error", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !imageEmoteExtensions[ext] {
			slog.Warn("Skipping unsupported emote file", "file", entry.Name(), "extension", ext)
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ext)
		// Sanitize: only allow alphanumeric and dashes
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(name) {
			slog.Warn("Skipping emote file with invalid name", "file", entry.Name())
			continue
		}

		src := "/static/emotes/" + entry.Name()
		emoteMap[name] = `<img src="` + src + `" alt=":` + template.HTMLEscapeString(name) + `:" class="emote">`
		slog.Info("Loaded image emote", "name", name, "file", entry.Name())
	}
}

// ReplaceEmotes replaces :name: tokens in text with their corresponding emoji or HTML.
// Unknown tokens are left unchanged. All non-emote content is HTML-escaped for safety;
// only the matched emote patterns are rendered as raw HTML (e.g., <img> tags).
func ReplaceEmotes(text string) template.HTML {
	// First, HTML-escape the entire text to neutralize any unsafe markup.
	escaped := template.HTMLEscapeString(text)
	// Then, find emote patterns in the escaped text and replace them with safe HTML.
	return template.HTML(emoteRegex.ReplaceAllStringFunc(escaped, func(match string) string {
		name := match[1 : len(match)-1] // strip leading and trailing ':'
		if replacement, ok := emoteMap[name]; ok {
			return replacement // image emote tags are pre-built safe HTML
		}
		return match // unknown tokens stay escaped (e.g., ":foobar:" → ":foobar:")
	}))
}