// Package app holds the write flows shared by the CLI and the dashboard:
// editing targets, add, fork, remove, purge, enable/disable, new, adopt,
// update, apply, the trash and the git backup. The CLI and the dashboard
// only gather input and format results; every change to a library goes
// through here, so both front ends behave the same.
//
// Nothing here deletes user data: anything taken out of the way goes to
// the library trash. EmptyTrash is the one exception and its callers only
// offer it to a person at a terminal.
package app

import (
	"fmt"
	"strings"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/targets"
)

// Level classifies a progress message.
type Level int

const (
	// Info is plain progress ("fetching owner/repo").
	Info Level = iota
	// Done reports a finished step ("copied foo → skills/foo").
	Done
	// Detail is a secondary line under a step ("moved aside …").
	Detail
	// Warn is a problem the operation worked around.
	Warn
	// Fail is a step that failed while the operation went on.
	Fail
	// Problem is a per-entry problem found while resolving the library
	// (name clash, unknown target, pack not fetched). Each is reported
	// once per session.
	Problem
)

// Session is an open library. It is not safe for concurrent use; open one
// per operation.
type Session struct {
	Lib *library.Library
	Reg *targets.Registry
	// Log receives progress and warnings; nil discards them.
	Log func(Level, string)

	warned map[string]bool
}

// Open loads the library at root and the target registry, including the
// manifest's custom targets.
func Open(root string) (*Session, error) {
	lib, err := library.Open(root)
	if err != nil {
		return nil, err
	}
	custom := map[string]struct{ Path, Mode string }{}
	for n, c := range lib.Manifest.Targets.Custom {
		custom[n] = struct{ Path, Mode string }{c.Path, c.Mode}
	}
	reg, err := targets.NewRegistry(custom)
	if err != nil {
		return nil, err
	}
	engine.SetRegistry(reg)
	return &Session{Lib: lib, Reg: reg}, nil
}

func (s *Session) logf(l Level, format string, a ...any) {
	if s.Log != nil {
		s.Log(l, fmt.Sprintf(format, a...))
	}
}

// Resolve computes the library's entries. Per-entry problems go to Log as
// Problem, each once per session.
func (s *Session) Resolve() ([]engine.Entry, error) {
	entries, warnings, err := engine.Resolve(s.Lib, s.Reg)
	for _, w := range warnings {
		if s.warned == nil {
			s.warned = map[string]bool{}
		}
		if !s.warned[w] {
			s.warned[w] = true
			s.logf(Problem, "%s", w)
		}
	}
	return entries, err
}

// Ref names the skill an operation acts on: either a key as a person or an
// agent types it, or an exact library entry (what a dashboard row shows).
type Ref struct {
	// Key is a link name, an upstream name, or a qualified
	// "<owner/repo|alias|skills|private|app>:<name>".
	Key string
	// Kind, Origin and Name, when Kind is set, select exactly one entry.
	// Rows are never matched by link name alone: a vendored tdd and your
	// own skills/tdd may both exist when their targets do not overlap.
	Kind   engine.Kind
	Origin string
	Name   string
	// Pack, when set, names a pack (owner/repo or alias) instead of a skill.
	Pack string
}

// KeyRef refers to whatever key names (a skill, or failing that a pack).
func KeyRef(key string) Ref { return Ref{Key: key} }

// EntryRef refers to one exact entry.
func EntryRef(kind engine.Kind, origin, name string) Ref {
	return Ref{Kind: kind, Origin: origin, Name: name, Key: name}
}

// PackRef refers to a pack.
func PackRef(repo string) Ref { return Ref{Pack: repo, Key: repo} }

func (r Ref) String() string {
	if r.Pack != "" {
		return r.Pack
	}
	return r.Key
}

// Entry finds the entry r refers to among entries, or (nil, nil).
func (s *Session) Entry(entries []engine.Entry, r Ref) (*engine.Entry, error) {
	if r.Pack != "" {
		return nil, nil
	}
	if r.Kind != "" {
		for i := range entries {
			e := &entries[i]
			if e.Kind == r.Kind && e.Origin == r.Origin && e.Name == r.Name {
				return e, nil
			}
		}
		return nil, nil
	}
	return s.Lookup(entries, r.Key)
}

// Lookup resolves a user-supplied skill key (see engine.Lookup); a pack
// alias is accepted in place of owner/repo before the colon, except the
// reserved "skills", "private", "app" and "vendor".
func (s *Session) Lookup(entries []engine.Entry, key string) (*engine.Entry, error) {
	if i := strings.LastIndex(key, ":"); i > 0 && !engine.ReservedQualifier(key[:i]) {
		if p := s.Lib.Manifest.FindPack(key[:i]); p != nil {
			key = p.Repo + key[i:]
		}
	}
	return engine.Lookup(entries, key)
}

// mustEntry is Entry that fails when nothing matches.
func (s *Session) mustEntry(entries []engine.Entry, r Ref) (*engine.Entry, error) {
	e, err := s.Entry(entries, r)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("no skill %q", r.String())
	}
	return e, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
