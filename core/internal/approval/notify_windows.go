//go:build windows

package approval

import (
	"fmt"
	"os/exec"

	"github.com/Hypership-Software/atlas/internal/schema"
)

// notifyDesktop shows a best-effort Windows balloon tip via WinForms (no extra
// modules required). Runs async and ignores all errors — notifications must
// never affect the gating decision.
func notifyDesktop(d schema.Descriptor) {
	msg := sanitizeMsg(fmt.Sprintf("%s %s needs approval", d.ToolClass, d.ToolRaw))
	script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms;`+
		`$n=New-Object System.Windows.Forms.NotifyIcon;`+
		`$n.Icon=[System.Drawing.SystemIcons]::Information;`+
		`$n.BalloonTipTitle='gated: approval needed';`+
		`$n.BalloonTipText='%s';$n.Visible=$true;$n.ShowBalloonTip(5000);`+
		`Start-Sleep -Seconds 6;$n.Dispose()`, msg)
	_ = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Run()
}
