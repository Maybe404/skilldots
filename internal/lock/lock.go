// Package lock reads and writes skills.lock, the machine-written record of
// what is vendored at which upstream commit and what is linked where.
package lock

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"

	"github.com/pelletier/go-toml/v2"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/safetext"
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
	// Upstream lists every skill name the upstream had at the last
	// `skd update`, so the next one can tell which skills are new.
	Upstream []string `toml:"upstream,omitempty"`
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
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := checkText(b); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if l.Packs == nil {
		l.Packs = map[string]*Pack{}
	}
	if l.Links == nil {
		l.Links = map[string]*Link{}
	}
	for _, repo := range sortedKeys(l.Packs) {
		if p := l.Packs[repo]; p != nil && p.Commit != "" && !commitRE.MatchString(p.Commit) {
			return nil, fmt.Errorf("%s: pack %q: commit %q is not a commit hash; restore the file (git checkout -- %s) or remove the line", path, repo, p.Commit, FileName)
		}
	}
	return l, nil
}

// commitRE is a commit as skd writes it: a hash, 7 to 64 hex digits
// (SHA-1 or SHA-256). The commit is passed to git, so anything else, an
// option such as --output=<file> above all, is refused when the lock is
// read.
var commitRE = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// checkText refuses a lock holding control characters, bidi controls or
// invalid UTF-8 anywhere, in a value or a key (pack and skill names,
// categories, link records and their paths): skd writes none, and every
// one of them ends up printed or used as a path, so the file was written
// to attack a terminal. Each place is named, quoted so it prints safely.
func checkText(b []byte) error {
	raw := map[string]any{}
	if err := toml.Unmarshal(b, &raw); err != nil {
		return err
	}
	var errs []error
	for _, p := range safetext.Doc(raw) {
		errs = append(errs, fmt.Errorf("%s has control characters; restore the file (git checkout -- %s) or remove the line", p, FileName))
	}
	return errors.Join(errs...)
}

func (l *Lock) Save(path string) error {
	b, err := toml.Marshal(l)
	if err != nil {
		return err
	}
	if err := checkText(b); err != nil {
		return fmt.Errorf("not saving %s: %w", FileName, err)
	}
	return fsutil.WriteAtomic(path, append([]byte("# Written by skilldots. Do not edit.\n"), b...))
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
