package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
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

// OlderSkd reports whether skd on this machine has kept no placed file
// yet: the links an skd older than the record left in agent
// directories may still be there unrecorded. The first library opened
// with this skd's own settings ends that (AdoptOlderAppLinks writes the
// file, recording what it can), as does pointing skd at another library
// (SealPlaced): from then on every link skd makes is recorded as it is
// made, and nothing outside the library is taken for skd's on what
// skills.toml and skills.lock say.
func OlderSkd() bool {
	p := placedFile()
	if p == "" {
		return false
	}
	_, err := os.Lstat(p)
	return os.IsNotExist(err)
}

// SealPlaced writes an empty placed file at p when there is none, so
// that OlderSkd is false from now on. skd clone, init and use call it
// before they point skd at a library: a library cloned from someone
// else is never the one an older skd made links from.
func SealPlaced(p string) {
	if p == "" {
		return
	}
	placedMu.Lock()
	defer placedMu.Unlock()
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		return
	}
	b, _ := json.MarshalIndent(placedDoc{Version: 1, Placed: map[string]placedEntry{}}, "", "  ")
	if os.MkdirAll(filepath.Dir(p), 0o755) == nil {
		fsutil.WriteAtomic(p, append(b, '\n'))
	}
}

// AdoptOlderAppLinks records in the placed file the links of the
// library's apps an older skd made, which kept no record of them, and
// writes the file even when there is none (OlderSkd is false after it).
// It returns how many it recorded. Only while OlderSkd, and only for
// entries as skills.toml has them now, before a command changes it, so
// that disabling the app or taking it out of skills.toml later removes
// them outright, as that skd did. The caller makes sure lib is the
// library this user's settings name (app.Open): see docs/design.md for
// why that, and not skills.toml and skills.lock, is the evidence.
//
// A link is recorded only when it is exactly what skd makes for an
// app's skill this library wants in that target (olderAppLink).
func AdoptOlderAppLinks(lib *library.Library, entries []Entry) int {
	placedMu.Lock()
	defer placedMu.Unlock()
	if !OlderSkd() {
		return 0
	}
	placed := loadPlaced()
	n := 0
	for i := range entries {
		e := &entries[i]
		if e.Kind != KindApp || e.Refused != "" || e.Missing || e.Conflict != "" {
			continue
		}
		for _, t := range e.Targets {
			dest := filepath.Join(t.Path, e.LinkName)
			if _, ok := placed[placedKey(dest)]; ok {
				continue
			}
			if olderAppLink(dest, lib.Lock.Links[t.Name+"/"+e.LinkName], e.Dir, lib) {
				placed[placedKey(dest)] = placedEntry{Link: e.Dir}
				n++
			}
		}
	}
	savePlaced(placed)
	return n
}

// olderAppLink reports whether dest is exactly the link skd makes for
// the app's skill at src: a symbolic link whose target is src as
// written, which the lock records as that link's source, src being an
// absolute, clean directory of its own (not itself a link) with a
// regular SKILL.md, not a place of keys (CheckAppDir), and neither src
// nor where it really is lies inside the library or in any agent's
// skill directory (where a link of the user's from one agent to
// another, or to skd's own link, leads; an app's skill is never there).
func olderAppLink(dest string, prev *lock.Link, src string, lib *library.Library) bool {
	if prev == nil || prev.Mode != "link" || prev.Source != src || !filepath.IsAbs(src) || filepath.Clean(src) != src {
		return false
	}
	if st, err := os.Lstat(src); err != nil || !st.IsDir() {
		return false
	}
	if st, err := os.Lstat(dest); err != nil || st.Mode()&os.ModeSymlink == 0 {
		return false
	}
	if got, err := os.Readlink(dest); err != nil || got != src {
		return false
	}
	if CheckAppDir(src) != nil {
		return false
	}
	real := fsutil.Resolve(src)
	places := []string{lib.Root}
	if reg := registryPtr.Load(); reg != nil {
		for _, t := range reg.All() {
			places = append(places, t.Path)
		}
	} else {
		return false
	}
	for _, d := range places {
		d = filepath.Clean(d)
		for _, p := range []string{src, real} {
			if fsutil.Inside(d, p, true) || fsutil.Inside(fsutil.Resolve(d), p, true) {
				return false
			}
		}
	}
	return true
}
