package meta

// version is overridden at build time via -ldflags "-X .../meta.version=...".
// It is a var (not const) because the linker can only rewrite vars; nothing
// mutates it at runtime.
var version = "0.0.0-dev"

func Version() string { return version }

// ProductName is the display name — intentionally the binary name until the
// trademark screen sets the final wordmark.
func ProductName() string { return "gated" }

func BinaryName() string { return "gated" }
