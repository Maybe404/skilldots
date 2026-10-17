package targets

import (
	"os"
	"path/filepath"
	"testing"
)

// A path through a symbolic link that leads nowhere yet cannot be known:
// once something appears where it points (a link skd itself makes in a
// built-in target, say) it leads there. It is refused.
func TestCheckPathRefusesALinkThatLeadsNowhere(t *testing.T) {
	home := sandbox(t)
	os.Symlink(filepath.Join(home, ".claude", "skills", "foo"), filepath.Join(home, "x"))
	os.MkdirAll(filepath.Join(home, "y"), 0o755)
	os.Symlink(filepath.Join(home, "nowhere"), filepath.Join(home, "y", "skills"))
	for _, p := range []string{"~/x/newdir", "~/x", "~/y/skills", "~/y/skills/a"} {
		if real, err := CheckPath(p, ""); err == nil {
			t.Errorf("CheckPath(%q) = %q, allowed", p, real)
		}
	}
	if _, err := CheckPath("~/y/other", ""); err != nil {
		t.Errorf("a path beside the link: %v", err)
	}
}

// The library given as a relative path is still the library.
func TestCheckPathRefusesTheLibraryGivenRelatively(t *testing.T) {
	home := sandbox(t)
	os.MkdirAll(filepath.Join(home, "lib", "skills"), 0o755)
	t.Chdir(home)
	if _, err := CheckPath("~/lib/skills", "lib"); err == nil {
		t.Error("a target inside the library given as lib")
	}
}
