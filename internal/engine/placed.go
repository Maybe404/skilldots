package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Maybe404/skilldots/internal/fsutil"
)

// The lock comes with the library, and a library may come from someone
// else (skd clone): its records say nothing skd can trust about what is
// on disk. A copy whose hash a forged record matches, or a link a forged
// record claims, may be the user's own. So skd also keeps its own record
// of what it placed in agent directories, per user and per machine,
// beside its settings and never in a library: the placed file. An entry
// is removed outright only when skd can tell it made it: a link into the
// library (a clean library-relative source, see libSource), or what the
// placed file records (a link with exactly that target, a copy exactly
// as it was written). Everything else that has to go is moved to the
// trash.

// placedEntry is what skd placed at one path: a link (its target as
// written) or a copy (its HashTree hash).
type placedEntry struct {
	Link string `json:"link,omitempty"`
	Hash string `json:"hash,omitempty"`
}

type placedDoc struct {
	Version int                    `json:"version"`
	Placed  map[string]placedEntry `json:"placed"`
}

// placedPath is the placed file; "" (tests that do not set it) keeps no
// record, so nothing is confirmed and nothing that is not a link into
// the library is removed outright.
var placedPath atomic.Pointer[string]

// placedMu serialises this process's reads and writes of the placed
// file. Another skd process writing it at the same time can lose an
// entry; the cost is a copy or link moved to the trash instead of
// removed, never the other way round.
var placedMu sync.Mutex

// SetPlacedFile sets where skd records what it placed (see placedEntry).
func SetPlacedFile(p string) { placedPath.Store(&p) }

// placedFile is the path of the placed file, "" when none is set.
func placedFile() string {
	if p := placedPath.Load(); p != nil {
		return *p
	}
	return ""
}

// placedKey is how a destination is keyed: absolute and clean.
func placedKey(dest string) string {
	if a, err := filepath.Abs(dest); err == nil {
		return a
	}
	return filepath.Clean(dest)
}

// loadPlaced reads the placed file. A missing or unreadable file is an
// empty record: nothing is confirmed.
func loadPlaced() map[string]placedEntry {
	out := map[string]placedEntry{}
	p := placedFile()
	if p == "" {
		return out
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return out
	}
	var d placedDoc
	if json.Unmarshal(b, &d) != nil || d.Placed == nil {
		return out
	}
	return d.Placed
}

// savePlaced writes m to the placed file. A failure is not an error of
// the apply: the record only lets skd remove its own things outright.
func savePlaced(m map[string]placedEntry) {
	p := placedFile()
	if p == "" {
		return
	}
	b, err := json.MarshalIndent(placedDoc{Version: 1, Placed: m}, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(p), 0o755) == nil {
		fsutil.WriteAtomic(p, append(b, '\n'))
	}
}

// confirms reports whether dest is exactly what the placed record m says
// skd put there: a symbolic link with the recorded target, or a
// directory holding exactly the recorded copy (fsutil.Pristine).
func confirms(m map[string]placedEntry, dest string) bool {
	e, ok := m[placedKey(dest)]
	if !ok {
		return false
	}
	st, err := os.Lstat(dest)
	if err != nil {
		return false
	}
	switch {
	case e.Link != "":
		got, err := os.Readlink(dest)
		return err == nil && st.Mode()&os.ModeSymlink != 0 && got == e.Link
	case e.Hash != "":
		return st.IsDir() && fsutil.Pristine(dest, e.Hash)
	}
	return false
}
