package install

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// binaryName is what the gate binary is installed as: the daemon and SessionStart
// shim launch it by this name from <home>/bin.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "gated.exe"
	}
	return "gated"
}

// sourceBinary resolves the binary to install from: the caller-provided path, else
// the running executable. "" when neither resolves, so install degrades to a note.
func sourceBinary(binaryPath string) string {
	if binaryPath != "" {
		return binaryPath
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return ""
}

// installBinary copies src into <home>/bin/<binaryName> and returns the installed
// path, so a rebuilt binary reaches the location the daemon and SessionStart shim
// launch from. replaced reports whether a copy happened; it is false when src is
// already the installed binary (copying a file onto itself would truncate it).
//
// A daemon launched from the destination holds an exclusive handle on Windows, so
// any running daemon is stopped first to release it; Init's Ensure restarts one
// from the freshly installed binary.
func installBinary(home, src string) (installed string, replaced bool, err error) {
	dest := filepath.Join(home, "bin", binaryName())
	if sameFile(src, dest) {
		return dest, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", false, err
	}
	_, _ = stopDaemon(home)
	if err := copyExecutable(src, dest); err != nil {
		return "", false, err
	}
	return dest, true, nil
}

func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

// copyExecutable writes src to dest atomically: stage to dest.new, then rename
// over dest so a crash mid-copy never leaves a torn binary.
func copyExecutable(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dest + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return replaceFile(tmp, dest)
}

// replaceFile renames tmp over dest, retrying briefly: on Windows a just-stopped
// daemon's handle on dest can linger a few milliseconds after the process dies, so
// the first rename may hit a sharing violation.
func replaceFile(tmp, dest string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(tmp, dest); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	os.Remove(tmp)
	return err
}
