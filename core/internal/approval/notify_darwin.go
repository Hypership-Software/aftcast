//go:build darwin

package approval

import (
	"fmt"
	"os/exec"

	"github.com/Hypership-Software/atlas/internal/schema"
)

// notifyDesktop posts a best-effort macOS notification via osascript. Runs async
// and ignores errors — notifications must never affect the gating decision.
func notifyDesktop(d schema.Descriptor) {
	msg := sanitizeMsg(fmt.Sprintf("%s %s needs approval", d.ToolClass, d.ToolRaw))
	script := fmt.Sprintf("display notification %q with title \"gated: approval needed\"", msg)
	_ = exec.Command("osascript", "-e", script).Run()
}
