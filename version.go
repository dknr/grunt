package grunt

var (
	// Version is the semantic version string, injected via -ldflags.
	Version = "dev"
	// Timestamp is the UTC build timestamp, injected via -ldflags.
	Timestamp = ""
	// Commit is the short git commit hash, injected via -ldflags.
	Commit = ""
)
