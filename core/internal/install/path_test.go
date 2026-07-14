package install

import "testing"

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
