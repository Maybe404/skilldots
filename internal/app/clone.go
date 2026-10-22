package app

import (
	"os"
	"path/filepath"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
)

// ErrNotLibrary is what CloneLibrary's error wraps when the repository
// has no skills.toml at its top.
var ErrNotLibrary = i18n.New("not a skilldots library")

// CloneLibrary clones the library at url into dir, which must not exist
// (skd clone, the desktop's import). A repository without a skills.toml
// at its top is not a library, most often someone's repository of
// skills: the clone is removed and the error, wrapping ErrNotLibrary,
// says so and names the skd add that uses it as a source instead.
//
// The clone is removed outright, not moved to a trash: there may be no
// library yet to hold one, and it is only what git just fetched into a
// directory this call made, nothing anyone wrote.
func CloneLibrary(url, dir string) error {
	if fsutil.Lexists(dir) {
		return i18n.Errorf("%s already exists", dir)
	}
	if _, err := library.Git("", "clone", "-q", "--", url, dir); err != nil {
		return err
	}
	if fsutil.Exists(filepath.Join(dir, manifest.FileName)) {
		return nil
	}
	skills, _, _ := pack.Scan(dir)
	if err := os.RemoveAll(dir); err != nil {
		return i18n.Errorf("%s is %w (no %s at its top); removing the clone failed too: %v", url, ErrNotLibrary, manifest.FileName, err)
	}
	if len(skills) == 0 {
		return i18n.Errorf("%s is %w (no %s at its top, and no SKILL.md in it); nothing was kept", url, ErrNotLibrary, manifest.FileName)
	}
	return i18n.Errorf("%s is %w but a repository of skills (no %s at its top); nothing was kept. To use its skills, add it as a source: skd add %s --all", url, ErrNotLibrary, manifest.FileName, url)
}
