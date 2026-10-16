// Package lock reads and writes skills.lock, the machine-written record of
// what is vendored at which upstream commit and what is linked where.
package lock

import (
	"os"
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
	return os.WriteFile(path, append([]byte("# Written by skilldots. Do not edit.\n"), b...), 0o644)
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
