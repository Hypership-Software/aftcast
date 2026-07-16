package meta

// version is overridden at build time via -ldflags "-X .../meta.version=...".
// It is a var (not const) because the linker can only rewrite vars; nothing
// mutates it at runtime.
var version = "0.0.0-dev"

func Version() string { return version }

func ProductName() string { return "Aftcast" }

func BinaryName() string { return "aftcast" }
