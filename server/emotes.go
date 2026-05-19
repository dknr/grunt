package server

import (
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// emoteMap maps emote names to their display value (emoji character or HTML).
// Access is guarded by emoteMu.
var emoteMap = map[string]string{
	"smile": "😀",
	"heart": "❤️",
}

// emoteMu guards read/write access to emoteMap.
var emoteMu sync.RWMutex

// emoteRegex matches :name: patterns, allowing hyphens in emote names.
var emoteRegex = regexp.MustCompile(`:[a-zA-Z0-9_-]+:`)

// imageEmoteExtensions lists supported image file extensions for custom emotes.
var imageEmoteExtensions = map[string]bool{
	".svg":  true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

// emoteDir is the resolved filesystem path for runtime emotes.
var emoteDir string

// textEmoteNames holds the set of hardcoded text emote names (without colons).
var textEmoteNames = map[string]bool{
	"smile": true,
	"heart": true,
}

func init() {
	emoteDir = resolveEmotePath()
	scanImageEmotes(emoteDir)
	StartEmoteWatcher()
}

// resolveEmotePath determines the emote directory using environment variables and XDG conventions.
func resolveEmotePath() string {
	// 1. Check explicit override
	if dir := os.Getenv("GRUNT_EMOTE_DIR"); dir != "" {
		return dir
	}

	// 2. Check XDG_DATA_HOME
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "grunt", "emotes")
	}

	// 3. Fallback to $HOME/.local/share/grunt/emotes
	home := os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, ".local", "share", "grunt", "emotes")
	}

	// 4. Absolute fallback (unlikely but safe)
	return "grunt/emotes"
}

// scanImageEmotes scans a directory for image emote files and updates emoteMap.
// It only adds/updates image emotes; text emoji entries are left untouched.
func scanImageEmotes(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("No emotes directory found or unable to read it", "path", dir, "error", err)
		return
	}

	imageEmoteNames := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !imageEmoteExtensions[ext] {
			slog.Debug("Skipping unsupported emote file", "file", entry.Name(), "extension", ext)
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ext)
		// Sanitize: only allow alphanumeric and dashes
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(name) {
			slog.Warn("Skipping emote file with invalid name", "file", entry.Name())
			continue
		}

		imageEmoteNames[name] = true
		src := "/emotes/" + entry.Name()
		emoteMap[name] = `<img src="` + src + `" alt=":` + template.HTMLEscapeString(name) + `:" class="emote">`
		slog.Debug("Loaded image emote", "name", name, "file", entry.Name())
	}

	// Remove any previously-loaded image emotes that no longer exist on disk.
	emoteMu.Lock()
	for key := range emoteMap {
		if !imageEmoteNames[key] && !textEmoteNames[key] {
			delete(emoteMap, key)
		}
	}
	count := len(emoteMap) - len(textEmoteNames)
	if count < 0 {
		count = 0
	}
	slog.Info("Emotes loaded", "path", dir, "count", count)
	emoteMu.Unlock()
}

// EmoteWatcher manages the fsnotify watcher for runtime emote changes.
type EmoteWatcher struct {
	watcher *fsnotify.Watcher
	dir     string
	stopCh  chan struct{}
}

var emoteWatcher *EmoteWatcher

// StartEmoteWatcher creates and starts a file watcher for the emote directory.
func StartEmoteWatcher() {
	if emoteDir == "" {
		return
	}

	// Check if directory exists; warn but don't fail if it doesn't.
	if _, err := os.Stat(emoteDir); os.IsNotExist(err) {
		slog.Warn("Emotes directory does not exist (file watcher disabled)", "path", emoteDir)
		return
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("Failed to create emote watcher", "error", err)
		return
	}

	if err := w.Add(emoteDir); err != nil {
		slog.Error("Failed to watch emotes directory", "path", emoteDir, "error", err)
		w.Close()
		return
	}

	watcher := &EmoteWatcher{
		watcher: w,
		dir:     emoteDir,
		stopCh:  make(chan struct{}),
	}
	emoteWatcher = watcher

	go watcher.run()
	slog.Info("Started emote file watcher", "path", emoteDir)
}

// run is the main event loop for the emote watcher.
func (w *EmoteWatcher) run() {
	for {
		select {
		case <-w.stopCh:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			filename := filepath.Base(event.Name)
			ext := strings.ToLower(filepath.Ext(filename))

			// Only care about known image emote extensions
			if !imageEmoteExtensions[ext] {
				continue
			}

			// Build the name from the filename (strip extension)
			name := strings.TrimSuffix(filename, ext)
			if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(name) {
				continue
			}

			switch {
			case event.Has(fsnotify.Create):
				slog.Info("Emote added", "name", name, "file", filename)
				w.reload()
			case event.Has(fsnotify.Write):
				w.reload()
			case event.Has(fsnotify.Remove), event.Has(fsnotify.Rename):
				slog.Info("Emote removed", "name", name, "file", filename)
				w.reload()
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("Emote watcher error", "error", err)
		}
	}
}

// reload rescans the emote directory and updates the map.
func (w *EmoteWatcher) reload() {
	scanImageEmotes(w.dir)
}

// Close stops the file watcher and releases resources.
func (w *EmoteWatcher) Close() {
	close(w.stopCh)
	w.watcher.Close()
	slog.Info("Stopped emote file watcher")
}

// GetEmoteDir returns the resolved emote directory path.
func GetEmoteDir() string {
	return emoteDir
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
		emoteMu.RLock()
		defer emoteMu.RUnlock()
		if replacement, ok := emoteMap[name]; ok {
			return replacement // image emote tags are pre-built safe HTML
		}
		return match // unknown tokens stay escaped (e.g., ":foobar:" → ":foobar:")
	}))
}

// HandleRuntimeEmotes serves runtime-emote files from the filesystem.
// This handler is used for emotes loaded from disk rather than the embedded FS.
func HandleRuntimeEmotes(w http.ResponseWriter, r *http.Request) {
	if emoteDir == "" {
		http.NotFound(w, r)
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/emotes/")
	if filename == "" || strings.Contains(filename, "..") {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(emoteDir, filename)

	// Ensure the resolved path is within emoteDir (directory traversal protection)
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, emoteDir+"/") && cleanPath != emoteDir {
		http.NotFound(w, r)
		return
	}

	switch {
	case strings.HasSuffix(filename, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	case strings.HasSuffix(filename, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(filename, ".gif"):
		w.Header().Set("Content-Type", "image/gif")
	case strings.HasSuffix(filename, ".webp"):
		w.Header().Set("Content-Type", "image/webp")
	}

	http.ServeFile(w, r, cleanPath)
}