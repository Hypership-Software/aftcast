package install

import (
	"path/filepath"
	"testing"
)

func TestAddToPath(t *testing.T) {
	next, changed := addToPath(`C:\a;C:\b`, `C:\c`)
	if !changed || next != `C:\a;C:\b;C:\c` {
		t.Fatalf("add: got %q changed=%v", next, changed)
	}
	if _, changed := addToPath(`C:\a;C:\c`, `C:\c`); changed {
		t.Fatal("adding an existing dir must be a no-op")
	}
	if _, changed := addToPath(`C:\a;C:\c\`, `C:\c`); changed {
		t.Fatal("trailing-slash variant must count as present")
	}
	if next, _ := addToPath("", `C:\c`); next != `C:\c` {
		t.Fatalf("empty PATH: got %q", next)
	}
}

func TestRemoveFromPath(t *testing.T) {
	if got := removeFromPath(`C:\a;C:\c;C:\b`, `C:\c`); got != `C:\a;C:\b` {
		t.Fatalf("remove: got %q", got)
	}
}

func TestShellProfilePath(t *testing.T) {
	home := filepath.Join("testdata", "home")
	tests := []struct {
		name     string
		shell    string
		existing []string
		want     string
	}{
		{name: "fresh zsh", shell: "/bin/zsh", want: ".zshrc"},
		{name: "bash prefers bashrc", shell: "/bin/bash", existing: []string{".bashrc", ".bash_profile"}, want: ".bashrc"},
		{name: "bash uses bash profile", shell: "/bin/bash", existing: []string{".bash_profile"}, want: ".bash_profile"},
		{name: "fresh bash", shell: "/bin/bash", want: ".profile"},
		{name: "unknown shell uses existing zshrc", shell: "/usr/local/bin/fish", existing: []string{".zshrc"}, want: ".zshrc"},
		{name: "unknown shell falls back to profile", shell: "/usr/local/bin/fish", want: ".profile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := make(map[string]bool, len(tt.existing))
			for _, name := range tt.existing {
				existing[filepath.Join(home, name)] = true
			}
			got := shellProfilePath(home, tt.shell, func(path string) bool { return existing[path] })
			if want := filepath.Join(home, tt.want); got != want {
				t.Fatalf("shellProfilePath = %q, want %q", got, want)
			}
		})
	}
}
