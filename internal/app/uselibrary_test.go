package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Maybe404/skilldots/internal/config"
)

// useLibraryHome gives the test its own settings directory (macOS takes
// it from HOME, others from XDG_CONFIG_HOME) and returns a library path.
func useLibraryHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	lib := filepath.Join(home, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	return lib
}

func placedExists(t *testing.T) bool {
	t.Helper()
	_, err := os.Lstat(PlacedFile())
	return err == nil
}

func TestUseLibrarySealsWhenNoLibraryIsRecorded(t *testing.T) {
	lib := useLibraryHome(t)
	if err := UseLibrary(lib); err != nil {
		t.Fatal(err)
	}
	if !placedExists(t) {
		t.Fatal("using a library with none recorded left the window for an older skd's links open")
	}
	if got := config.Get(config.Library); got == "" {
		t.Fatal("the library was not recorded")
	}
}

func TestUseLibraryKeepsTheWindowForTheRecordedLibrary(t *testing.T) {
	lib := useLibraryHome(t)
	if err := os.MkdirAll(filepath.Dir(config.Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.Path(), []byte("library = '"+lib+"'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UseLibrary(lib); err != nil {
		t.Fatal(err)
	}
	if placedExists(t) {
		t.Fatal("using the recorded library again sealed its window")
	}
}

func TestUseLibraryWithBrokenSettingsSealsNothing(t *testing.T) {
	lib := useLibraryHome(t)
	if err := os.MkdirAll(filepath.Dir(config.Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.Path(), []byte("library = [[[\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UseLibrary(lib); err == nil {
		t.Fatal("a settings file that cannot be read was not reported")
	}
	if placedExists(t) {
		t.Fatal("a settings file that cannot be read still sealed the window")
	}
}
