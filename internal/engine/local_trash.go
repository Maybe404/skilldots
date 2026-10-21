package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
)

var localTrashMu sync.Mutex

func localTrashPath() string { return filepath.Join(filepath.Dir(config.Path()), "local-trash.json") }
func readLocalTrash() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(localTrashPath())
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &m)
	if m == nil {
		m = map[string]string{}
	}
	return m, err
}

// TrashLocal records local ownership outside the clonable library. A forged
// .origin in a cloned library must not create skills in skills/ or private/.
func TrashLocal(lib *library.Library, src string) (string, error) {
	// The records must be readable before anything moves.
	localTrashMu.Lock()
	_, err := readLocalTrash()
	localTrashMu.Unlock()
	if err != nil {
		return "", err
	}
	// Trash forgets any record of an earlier entry at dest (taking the lock).
	dest, err := Trash(lib, src)
	if err != nil {
		return dest, err
	}
	localTrashMu.Lock()
	defer localTrashMu.Unlock()
	records, err := readLocalTrash()
	if err != nil {
		return dest, i18n.Errorf("%s was moved to %s, but where it came from could not be recorded: %w", src, dest, err)
	}
	records[fsutil.Resolve(dest)] = src
	b, err := json.Marshal(records)
	if err != nil {
		return dest, err
	}
	if err = fsutil.WriteAtomic(localTrashPath(), b); err != nil {
		return dest, i18n.Errorf("%s was moved to %s, but where it came from could not be recorded: %w", src, dest, err)
	}
	return dest, nil
}
func ownsLocalTrash(entry, origin string) bool {
	localTrashMu.Lock()
	defer localTrashMu.Unlock()
	records, err := readLocalTrash()
	return err == nil && records[fsutil.Resolve(entry)] == origin
}

// forgetLocalTrash drops the provenance records of the entries at p or
// under it (deleted for good).
func forgetLocalTrash(p string, under bool) {
	localTrashMu.Lock()
	defer localTrashMu.Unlock()
	records, err := readLocalTrash()
	if err != nil {
		return
	}
	key := fsutil.Resolve(p)
	n := len(records)
	for k := range records {
		if k == key || (under && strings.HasPrefix(k, key+string(filepath.Separator))) {
			delete(records, k)
		}
	}
	if len(records) == n {
		return
	}
	if b, err := json.Marshal(records); err == nil {
		_ = fsutil.WriteAtomic(localTrashPath(), b)
	}
}

func forgetLocalTrashUnder(dir string) { forgetLocalTrash(dir, true) }

// ForgetTrashEntry drops everything skd remembers on this machine about
// the trash entry at p: its note (trash-notes.json) and, for a skill of
// the user's, its provenance record (local-trash.json). Called when the
// entry is deleted for good, and before another takes its place.
func ForgetTrashEntry(p string) error {
	err := DropTrashNote(p)
	forgetLocalTrash(p, false)
	return err
}
