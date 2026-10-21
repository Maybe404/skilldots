package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
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
	// ReasonBroken: a broken link in an agent's skill directory that adopt
	// cleared away.
	ReasonBroken = "broken"
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

// trashNotesMu guards the notes in this process; a lock file beside them
// (trash-notes.lock) guards them against other processes: the desktop
// app and an agent's skd may set aside, restore or delete at once.
var trashNotesMu sync.Mutex

func trashNotesPath() string {
	return filepath.Join(filepath.Dir(config.Path()), "trash-notes.json")
}

// BrokenNotesError is a notes file that was not valid JSON: it is kept
// under Backup and a new one is started.
type BrokenNotesError struct {
	Path, Backup string
	Err          error
}

func (e *BrokenNotesError) Error() string {
	return i18n.Sprintf("%s was not valid JSON (%v); it is kept as %s and a new one is started: entries set aside before are restored without being selected again", e.Path, e.Err, e.Backup)
}

// readTrashNotes reads the notes. A file that is not valid JSON is an
// error, and is left as it is.
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
		return nil, &BrokenNotesError{Path: trashNotesPath(), Err: err}
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

// editTrashNotes reads the notes, lets edit change them (it reports
// whether it did) and writes them back, all under both locks, so a note
// another process writes meanwhile is not lost. A file that is not valid
// JSON is never written over: it is renamed aside first, and the error
// says where it went.
func editTrashNotes(edit func(m map[string]*TrashNote) bool) error {
	trashNotesMu.Lock()
	defer trashNotesMu.Unlock()
	defer pack.LockFile(strings.TrimSuffix(trashNotesPath(), ".json") + ".lock")()
	m, err := readTrashNotes()
	var broken *BrokenNotesError
	if errors.As(err, &broken) {
		broken.Backup = trashNotesPath() + ".broken-" + time.Now().Format("20060102-150405")
		if rerr := os.Rename(trashNotesPath(), broken.Backup); rerr != nil {
			return rerr
		}
		m, err = map[string]*TrashNote{}, nil
	}
	if err != nil {
		return err
	}
	if edit(m) {
		if werr := writeTrashNotes(m); werr != nil {
			return werr
		}
	}
	if broken != nil {
		return broken
	}
	return nil
}

// notify, when set (SetNotify), is told what a person should know about
// a step that went on regardless: a broken notes file set aside while
// something was moved to the trash.
var notify func(msg string)

// SetNotify sets where engine sends such warnings (app.Open: the
// session's log).
func SetNotify(f func(msg string)) { notify = f }

func warnf(err error) {
	if err != nil && notify != nil {
		notify(err.Error())
	}
}

// noteKey is the key of the entry at p in the notes: its resolved path.
func noteKey(p string) string { return fsutil.Resolve(p) }

// SetTrashNote records n for the trash entry at path (where Trash moved
// it), replacing what was recorded. A note that cannot be written only
// costs the restore its selection: the entry itself is in the trash.
func SetTrashNote(path string, n *TrashNote) error {
	return editTrashNotes(func(m map[string]*TrashNote) bool {
		m[noteKey(path)] = n
		return true
	})
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

// DropTrashNote forgets the note of the entry at path (restored, or
// deleted for good). Its error is a warning: a notes file that was not
// valid JSON was set aside (BrokenNotesError).
func DropTrashNote(path string) error {
	return editTrashNotes(func(m map[string]*TrashNote) bool {
		if m[noteKey(path)] == nil {
			return false
		}
		delete(m, noteKey(path))
		return true
	})
}

// dropTrashNotesUnder forgets the notes of every entry in dir (the trash
// of a library that was emptied).
func dropTrashNotesUnder(dir string) error {
	root := noteKey(dir)
	return editTrashNotes(func(m map[string]*TrashNote) bool {
		n := len(m)
		for k := range m {
			if strings.HasPrefix(k, root+string(filepath.Separator)) {
				delete(m, k)
			}
		}
		return len(m) != n
	})
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
