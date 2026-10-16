package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/safetext"
)

// TrashDir is the library folder where skilldots parks anything it takes
// out of the way. Nothing is ever deleted outright; `skd trash empty` is
// the only destructive step and it is reserved for a person at a terminal.
const TrashDir = ".trash"

// TrashEntry is one parked directory.
type TrashEntry struct {
	ID     string    `json:"id"` // "<batch>/<name>"
	Name   string    `json:"name"`
	Origin string    `json:"origin"` // where it came from
	Path   string    `json:"path"`   // where it is now
	When   time.Time `json:"when"`
}

// trashBatch is the per-run folder so one command's moves stay together.
var trashBatch string

func batch() string {
	if trashBatch == "" {
		trashBatch = time.Now().Format("20060102-150405")
	}
	return trashBatch
}

// Trash moves src into the library trash and records its origin.
// Symlinks are simply removed: they hold no data.
func Trash(lib *library.Library, src string) (string, error) {
	st, err := os.Lstat(src)
	if err != nil {
		return "", err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return "", os.Remove(src)
	}
	dir := filepath.Join(lib.Root, TrashDir, batch())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Base(src)
	dest := filepath.Join(dir, name)
	for i := 2; fsutil.Lexists(dest); i++ {
		dest = filepath.Join(dir, fmt.Sprintf("%s-%d", name, i))
	}
	if err := move(src, dest); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest+".origin", []byte(src+"\n"), 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

// rename is os.Rename, replaceable in tests.
var rename = os.Rename

// move renames src to dst. Only when that fails because they are on
// different devices does it fall back to an exact copy (everything,
// including .git, node_modules and symlinks) followed by removing src.
// Any other failure is returned with src untouched.
func move(src, dst string) error {
	err := rename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := fsutil.CopyAll(src, dst); err != nil {
		os.RemoveAll(dst) // a partial copy; src is still complete
		return fmt.Errorf("moving %s across devices: %w", src, err)
	}
	return os.RemoveAll(src)
}

// ListTrash returns every parked directory, newest first.
func ListTrash(lib *library.Library) ([]TrashEntry, error) {
	root := filepath.Join(lib.Root, TrashDir)
	batches, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []TrashEntry
	for _, b := range batches {
		if !b.IsDir() {
			continue
		}
		items, _ := os.ReadDir(filepath.Join(root, b.Name()))
		for _, it := range items {
			if !it.IsDir() {
				continue
			}
			p := filepath.Join(root, b.Name(), it.Name())
			origin, _ := os.ReadFile(p + ".origin")
			when, _ := time.ParseInLocation("20060102-150405", b.Name(), time.Local)
			out = append(out, TrashEntry{ID: b.Name() + "/" + it.Name(), Name: it.Name(),
				Origin: strings.TrimSpace(string(origin)), Path: p, When: when})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// Restore moves a parked directory back to its origin.
func Restore(lib *library.Library, id string) (string, error) {
	p := filepath.Join(lib.Root, TrashDir, filepath.FromSlash(id))
	origin, err := os.ReadFile(p + ".origin")
	if err != nil {
		return "", fmt.Errorf("no trash entry %q", id)
	}
	dest := strings.TrimSpace(string(origin))
	// skd records where it took a directory from, and never takes one
	// from a path with control characters: such a record was written by
	// someone else, and restoring would create that name.
	if safetext.Has(dest) {
		return "", fmt.Errorf("trash entry %q: where it came from (%q) has control characters; restore it by hand from %q", id, dest, p)
	}
	if fsutil.Lexists(dest) {
		return "", fmt.Errorf("%s already exists; remove it first (skd disable/remove) or restore by hand from %s", dest, p)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := move(p, dest); err != nil {
		return "", err
	}
	os.Remove(p + ".origin")
	pruneBatch(filepath.Dir(p))
	return dest, nil
}

// EmptyTrash deletes everything parked. Irreversible.
func EmptyTrash(lib *library.Library) (int, error) {
	entries, err := ListTrash(lib)
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(filepath.Join(lib.Root, TrashDir)); err != nil {
		return 0, err
	}
	return len(entries), nil
}

func pruneBatch(dir string) {
	if items, err := os.ReadDir(dir); err == nil && len(items) == 0 {
		os.Remove(dir)
	}
}
