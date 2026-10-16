// Package lock reads and writes skills.lock, the machine-written record of
// what is vendored at which upstream commit and what is linked where.
package lock

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/pelletier/go-toml/v2"
)

const FileName = "skills.lock"

type Lock struct {
	Version int              `toml:"version"`
	Packs   map[string]*Pack `toml:"packs"`
	// Links records every file skilldots placed in a target, keyed by
	// "<target>/<link name>", so apply/remove only ever touch their own work.
	Links map[string]*Link `toml:"links"`
}

type Pack struct {
	Commit string `toml:"commit"`
	Ref    string `toml:"ref,omitempty"`
	// Skills maps upstream skill name to its vendored content hash.
	Skills map[string]*Skill `toml:"skills"`
}

type Skill struct {
	Category string `toml:"category,omitempty"`
	Hash     string `toml:"hash"`
}

type Link struct {
	// Mode is "link" or "copy".
	Mode string `toml:"mode"`
	// Source is the library-relative (or absolute, for apps) path linked.
	Source string `toml:"source"`
	// Hash is set for copies so unmodified copies can be identified later.
	Hash string `toml:"hash,omitempty"`
	// Path is the absolute location written, so the entry can still be
	// cleaned up after its target disappears from the manifest.
	Path string `toml:"path,omitempty"`
}

func Load(path string) (*Lock, error) {
	l := &Lock{Version: 1, Packs: map[string]*Pack{}, Links: map[string]*Link{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(b, l); err != nil {
		return nil, err
	}
	if l.Packs == nil {
		l.Packs = map[string]*Pack{}
	}
	if l.Links == nil {
		l.Links = map[string]*Link{}
	}
	return l, nil
}

func (l *Lock) Save(path string) error {
	b, err := toml.Marshal(l)
	if err != nil {
		return err
	}
	return writeAtomic(path, append([]byte("# Written by skilldots. Do not edit.\n"), b...))
}

// writeAtomic replaces path with data through a temporary file and a
// rename, so an interrupted save never leaves a torn lock. A symlinked
// lock is written through and an existing file keeps its permissions.
// (Same as manifest's; the two packages do not depend on each other.)
func writeAtomic(path string, data []byte) error {
	if st, err := os.Lstat(path); err == nil && st.Mode()&os.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return os.WriteFile(path, data, 0o644)
		}
		path = real
	}
	perm := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, perm)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp) // our own temporary file, never the user's
	}
	return err
}

// LinkKeys returns link keys sorted, for deterministic output.
func (l *Lock) LinkKeys() []string {
	keys := make([]string, 0, len(l.Links))
	for k := range l.Links {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
