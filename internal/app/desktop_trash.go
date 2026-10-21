package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/i18n"
)

// TrashUsage is what the trash holds: the entries Trash lists, the disk
// space of each (by ID) and of the whole trash folder, which is what
// EmptyTrash deletes.
type TrashUsage struct {
	Entries []engine.TrashEntry `json:"entries"`
	Sizes   map[string]int64    `json:"sizes"`
	Total   int64               `json:"total"`
}

// TrashUsage lists the trash with sizes. Symbolic links count as
// themselves; nothing they point at is read.
func (s *Session) TrashUsage() (*TrashUsage, error) {
	entries, err := s.Trash()
	if err != nil {
		return nil, err
	}
	out := &TrashUsage{Entries: entries, Sizes: map[string]int64{}}
	if out.Entries == nil {
		out.Entries = []engine.TrashEntry{}
	}
	for _, e := range entries {
		if !e.Unsafe {
			out.Sizes[e.ID] = diskSize(e.Path)
		}
	}
	out.Total = diskSize(filepath.Join(s.Lib.Root, engine.TrashDir))
	return out, nil
}

// diskSize adds up the sizes of everything under p (p itself when it is
// a file or a link), without following links.
func diskSize(p string) int64 {
	var n int64
	filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := d.Info(); err == nil && !d.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}

// DeleteTrashEntry permanently deletes one trash entry, as `skd trash`
// lists it, and its record. Like EmptyTrash it cannot be undone: callers
// offer it only to a person, after asking. An entry whose name skd does
// not show (Unsafe) is left for the person to look at by hand.
func (s *Session) DeleteTrashEntry(id string) error {
	entries, err := s.Trash()
	if err != nil {
		return err
	}
	var e *engine.TrashEntry
	for i := range entries {
		if entries[i].ID == id {
			e = &entries[i]
		}
	}
	if e == nil {
		return i18n.Errorf("no trash entry %q", id)
	}
	if e.Unsafe {
		return i18n.Errorf("trash entry %q is not named as skd names what it sets aside; look at it by hand in the library's %s", id, engine.TrashDir)
	}
	batch, name, ok := strings.Cut(id, "/")
	root := filepath.Join(s.Lib.Root, engine.TrashDir)
	p := filepath.Join(root, batch, name)
	if !ok || e.Path != p || filepath.Dir(filepath.Dir(p)) != root {
		return i18n.Errorf("no trash entry %q", id)
	}
	// The trash and the batch must be directories of their own: deleting
	// through a link would delete wherever it points.
	for _, d := range []string{root, filepath.Dir(p)} {
		st, err := os.Lstat(d)
		if err != nil {
			return err
		}
		if !st.IsDir() {
			return i18n.Errorf("%s is not a directory; skd does not follow it", d)
		}
	}
	if err := os.RemoveAll(p); err != nil {
		return err
	}
	os.Remove(p + ".origin")
	// What skd remembered about it goes with it: a later entry of the same
	// name in that batch must not look like this one.
	engine.ForgetTrashEntry(p)
	if items, err := os.ReadDir(filepath.Dir(p)); err == nil && len(items) == 0 {
		os.Remove(filepath.Dir(p))
	}
	return nil
}
