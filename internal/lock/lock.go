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
	"github.com/Maybe404/skilldots/internal/i18n"
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
	// Shelved is set while an update of this skill waits for its
	// conflicts to be resolved: the upstream commit whose version could
	// not be merged with the local changes. Hash stays the version the
	// local changes were made on until `skd resolve` finishes. An older
	// skd ignores these fields.
	Shelved string `toml:"shelved,omitempty"`
	// Upstream is the content hash of the upstream version being merged
	// (Shelved's); `skd resolve --continue` makes it Hash.
	Upstream string `toml:"upstream,omitempty"`
	// Conflicts are the files, relative to the skill's directory, that
	// both sides changed in ways that do not merge.
	Conflicts []string `toml:"conflicts,omitempty"`
	// Resolving is set from `skd resolve <skill>` until --continue or
	// --abort: the directory holds the merge with conflict markers, and
	// Backup is the trash ID of the directory as it was before.
	Resolving bool   `toml:"resolving,omitempty"`
	Backup    string `toml:"backup,omitempty"`
	// Contribution is the user's change to this skill as last sent
	// upstream (skd contribute): kept until an update brings upstream's
	// version to what the user has (the change landed).
	Contribution *Contribution `toml:"contribution,omitempty"`
	// Draft is set for a skill created in the user's own repository
	// (skd new --in) that is not there yet: it lives only in vendor/
	// until skd contribute pushes it, at Path (slash-separated, relative
	// to the repository's top). Hash is "" meanwhile. An older skd
	// ignores both, and drops them when it saves the lock: a skill with
	// no hash is a draft all the same (IsDraft).
	Draft bool   `toml:"draft,omitempty"`
	Path  string `toml:"path,omitempty"`
}

// IsDraft reports whether the skill is a draft: never vendored from
// upstream, only made here (skd new --in).
func (s *Skill) IsDraft() bool { return s != nil && (s.Draft || s.Hash == "") }

// Contribution is a change to a vendored skill sent upstream.
type Contribution struct {
	// Kind is "pr" (a pull request) or "push" (pushed to the branch).
	Kind string `toml:"kind"`
	// URL is the pull request's; Number its number.
	URL    string `toml:"url,omitempty"`
	Number int    `toml:"number,omitempty"`
	// Branch is the branch pushed to (the pull request's head branch).
	Branch string `toml:"branch,omitempty"`
	// State is "open", "merged" or "closed" for a pull request, "pushed"
	// for a push.
	State string `toml:"state"`
	// At is when it was sent (RFC 3339).
	At string `toml:"at,omitempty"`
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
			return nil, i18n.Errorf("%s: pack %q: commit %q is not a commit hash; restore the file (git checkout -- %s) or remove the line", path, repo, p.Commit, FileName)
		}
		if p := l.Packs[repo]; p != nil {
			for _, n := range sortedKeys(p.Skills) {
				if s := p.Skills[n]; s != nil && s.Shelved != "" && !commitRE.MatchString(s.Shelved) {
					return nil, i18n.Errorf("%s: pack %q: skill %q: shelved %q is not a commit hash; restore the file (git checkout -- %s) or remove the line", path, repo, n, s.Shelved, FileName)
				}
			}
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

// known are the places of the lock skd reads.
var known = []string{
	"packs.*", "packs.*.commit", "packs.*.ref", "packs.*.upstream",
	"packs.*.skills.*", "packs.*.skills.*.category", "packs.*.skills.*.hash",
	"packs.*.skills.*.shelved", "packs.*.skills.*.upstream", "packs.*.skills.*.conflicts",
	"packs.*.skills.*.resolving", "packs.*.skills.*.backup", "packs.*.skills.*.draft", "packs.*.skills.*.path", "packs.*.skills.*.contribution", "packs.*.skills.*.contribution.kind",
	"packs.*.skills.*.contribution.url", "packs.*.skills.*.contribution.branch", "packs.*.skills.*.contribution.state", "packs.*.skills.*.contribution.at",
	"links.*", "links.*.mode", "links.*.source", "links.*.hash", "links.*.path",
}

// checkText refuses a lock holding control characters, bidi controls or
// invalid UTF-8 in a place skd reads (known: pack and skill names,
// categories, link records and their paths): skd writes none, and every
// one of them ends up printed or used as a path, so the file was written
// to attack a terminal. Places this version does not know (a newer
// skd's fields) are refused only for an escape, BEL, DEL, C1, a bidi
// control or invalid UTF-8, like the manifest's. Each place is named,
// quoted so it prints safely.
func checkText(b []byte) error {
	raw := map[string]any{}
	if err := toml.Unmarshal(b, &raw); err != nil {
		return err
	}
	var errs []error
	for _, p := range safetext.DocSchema(raw, known) {
		errs = append(errs, i18n.Errorf("%s has control characters; restore the file (git checkout -- %s) or remove the line", p, FileName))
	}
	return errors.Join(errs...)
}

func (l *Lock) Save(path string) error {
	b, err := toml.Marshal(l)
	if err != nil {
		return err
	}
	if err := checkText(b); err != nil {
		return i18n.Errorf("not saving %s: %w", FileName, err)
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
