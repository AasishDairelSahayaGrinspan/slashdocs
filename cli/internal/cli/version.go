package cli

// Version, Commit, and ReleaseKey are set with -ldflags in release builds.
// ReleaseKey is the base64 ed25519 public key that signs CLI releases. A
// build without it installs no updates. See docs/releasing.md.
var (
	Version    = "dev"
	Commit     = ""
	ReleaseKey = ""
)
