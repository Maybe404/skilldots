package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
)

// Why a trash entry was set aside (TrashNote.Reason, TrashEntry.Reason).
const (
	// ReasonRemove: skd remove took the skill out of the library.
	ReasonRemove = "remove"
	// ReasonPurge: skd purge dropped the whole pack.
	ReasonPurge = "purge"
	// ReasonDeselected: the skill is no longer selected in skills.toml
	// (an update or a fork dropped it from vendor/).
	ReasonDeselected = "deselected"
	// ReasonReplaced: an update replaced a vendored copy that had changes.
	ReasonReplaced = "replaced"
	// ReasonInTheWay: something in an agent's skill directory was in the
	// way (apply --force, adopt). Not recorded: told by where it came from.
	ReasonInTheWay = "in-the-way"
)

// TrashNote is what skd remembers about an entry it moved to the trash:
// why, which skills it holds, and, for a vendored skill or pack, the
// pack's entry in skills.toml and skills.lock before it went, so that
// restoring it can select it again (app.Session.RestoreEntry).
//
// Notes are kept beside the settings file (trash-notes.json), per user
// and machine, never in a library: a cloned library's .trash is someone
// else's to write, and what restoring puts back into skills.toml must
// come from what skd itself saw here.
type TrashNote struct {
	Reason string   `json:"reason"`
	Skills []string `json:"skills,omitempty"`
	// Repo is the pack the entry was vendored from.
	Repo string `json:"repo,omitempty"`
	// Pack and Lock are the pack's records before the change.
	Pack *manifest.Pack `json:"pack,omitempty"`
	Lock *lock.Pack     `json:"lock,omitempty"`
}

var trashNotesMu sync.Mutex

func trashNotesPath() string {
	return filepath.Join(filepath.Dir(config.Path()), "trash-notes.json")
}

func readTrashNotes() (map[string]*TrashNote, error) {
	m := map[string]*TrashNote{}
	b, err := os.ReadFile(trashNotesPath())
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]*TrashNote{}
	}
	return m, nil
}

func writeTrashNotes(m map[string]*TrashNote) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(trashNotesPath()), 0o755); err != nil {
		return err
	}
	return fsutil.WriteAtomic(trashNotesPath(), b)
}

// noteKey is the key of the entry at p in the notes: its resolved path.
func noteKey(p string) string { return fsutil.Resolve(p) }

// SetTrashNote records n for the trash entry at path (where Trash moved
// it), replacing what was recorded. A note that cannot be written only
// costs the restore its selection: the entry itself is in the trash.
func SetTrashNote(path string, n *TrashNote) error {
	trashNotesMu.Lock()
	defer trashNotesMu.Unlock()
	m, err := readTrashNotes()
	if err != nil {
		m = map[string]*TrashNote{}
	}
	m[noteKey(path)] = n
	return writeTrashNotes(m)
}

// TrashNoteFor is the note recorded for the trash entry at path, or nil.
func TrashNoteFor(path string) *TrashNote {
	trashNotesMu.Lock()
	defer trashNotesMu.Unlock()
	m, err := readTrashNotes()
	if err != nil {
		return nil
	}
	return m[noteKey(path)]
}

// DropTrashNote forgets the note of the entry at path (restored).
func DropTrashNote(path string) {
	trashNotesMu.Lock()
	defer trashNotesMu.Unlock()
	m, err := readTrashNotes()
	if err != nil || m[noteKey(path)] == nil {
		return
	}
	delete(m, noteKey(path))
	_ = writeTrashNotes(m)
}

// dropTrashNotesUnder forgets the notes of every entry in dir (the trash
// of a library that was emptied).
func dropTrashNotesUnder(dir string) {
	trashNotesMu.Lock()
	defer trashNotesMu.Unlock()
	m, err := readTrashNotes()
	if err != nil {
		return
	}
	root := noteKey(dir)
	n := len(m)
	for k := range m {
		if strings.HasPrefix(k, root+string(filepath.Separator)) {
			delete(m, k)
		}
	}
	if len(m) != n {
		_ = writeTrashNotes(m)
	}
}

// TrashEntryPath is where the trash entry id is (see Restore for the
// checks).
func TrashEntryPath(lib *library.Library, id string) (string, error) { return entryPath(lib, id) }

// TrashID is the ID `skd trash` lists for the entry Trash moved to path.
func TrashID(lib *library.Library, path string) string {
	rel, err := filepath.Rel(filepath.Join(lib.Root, TrashDir), path)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}
