//go:build !windows && !darwin

package approval

import "github.com/Hypership-Software/atlas/internal/schema"

// notifyDesktop is a no-op on platforms without a blessed notifier (Linux is
// build/test-only here). A notify-send integration can land later if needed.
func notifyDesktop(schema.Descriptor) {}
