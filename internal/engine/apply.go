package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/targets"
)

// Op is one planned file-system change.
type Op struct {
	// Action: create, replace, remove, skip, keep.
	Action string
	Target targets.Target
	Name   string
	Dest   string
	Source string
	Mode   string
	Reason string
	entry  *Entry
}

// Plan is the diff between desired state and the lock/file system.
type Plan struct {
	Ops []Op
}

// Counts summarises the plan by action.
func (p Plan) Counts() map[string]int {
	c := map[string]int{}
	for _, o := range p.Ops {
		c[o.Action]++
	}
	return c
}

// Changes reports whether anything would be written.
func (p Plan) Changes() bool {
	for _, o := range p.Ops {
		if o.Action == "create" || o.Action == "replace" || o.Action == "remove" {
			return true
		}
	}
	return false
}

// MakePlan compares entries against the lock and disk.
func MakePlan(lib *library.Library, entries []Entry, force bool) Plan {
	var plan Plan
	desired := map[string]bool{}
	for i := range entries {
		e := &entries[i]
		for _, t := range e.Targets {
			dest := filepath.Join(t.Path, e.LinkName)
			key := t.Name + "/" + e.LinkName
			desired[key] = true
			op := Op{Target: t, Name: e.LinkName, Dest: dest, Source: e.Dir, Mode: t.Mode, entry: e}
			if e.Missing {
				op.Action, op.Reason = "skip", "source directory missing"
				plan.Ops = append(plan.Ops, op)
				continue
			}
			prev := lib.Lock.Links[key]
			switch {
			case !fsutil.Lexists(dest):
				op.Action = "create"
			case prev != nil && current(dest, prev, lib):
				if sameAs(dest, prev, e.Dir, lib) {
					op.Action = "keep"
				} else {
					op.Action, op.Reason = "replace", "source changed"
				}
			case prev != nil:
				// We placed it, but it was modified or replaced since.
				if force {
					op.Action, op.Reason = "replace", "modified since skilldots wrote it (--force)"
				} else {
					op.Action, op.Reason = "skip", "modified since skilldots wrote it; use --force"
				}
			default:
				// Exists and we never placed it.
				if isLinkTo(dest, e.Dir) {
					op.Action, op.Reason = "create", "adopting existing link"
				} else if force {
					op.Action, op.Reason = "replace", "unmanaged entry (--force)"
				} else {
					op.Action, op.Reason = "skip", "exists and not managed by skilldots; use --force"
				}
			}
			plan.Ops = append(plan.Ops, op)
		}
	}
	// Anything in the lock no longer desired gets removed.
	for _, key := range lib.Lock.LinkKeys() {
		if desired[key] {
			continue
		}
		prev := lib.Lock.Links[key]
		tname, name, _ := splitKey(key)
		t, err := regTarget(lib, tname)
		if err != nil {
			plan.Ops = append(plan.Ops, Op{Action: "skip", Name: name, Reason: err.Error()})
			continue
		}
		dest := filepath.Join(t.Path, name)
		op := Op{Target: t, Name: name, Dest: dest, Source: prev.Source, Mode: prev.Mode}
		switch {
		case !fsutil.Lexists(dest):
			op.Action, op.Reason = "remove", "already gone"
		case current(dest, prev, lib) || force:
			op.Action = "remove"
		default:
			op.Action, op.Reason = "skip", "modified since skilldots wrote it; use --force to remove"
		}
		plan.Ops = append(plan.Ops, op)
	}
	sort.SliceStable(plan.Ops, func(i, j int) bool {
		if plan.Ops[i].Target.Name != plan.Ops[j].Target.Name {
			return plan.Ops[i].Target.Name < plan.Ops[j].Target.Name
		}
		return plan.Ops[i].Name < plan.Ops[j].Name
	})
	return plan
}

// Apply executes the plan and updates the lock. The caller saves the library.
func Apply(lib *library.Library, plan Plan) error {
	for _, op := range plan.Ops {
		key := op.Target.Name + "/" + op.Name
		switch op.Action {
		case "create", "replace":
			if err := os.MkdirAll(op.Target.Path, 0o755); err != nil {
				return err
			}
			if fsutil.Lexists(op.Dest) {
				if err := os.RemoveAll(op.Dest); err != nil {
					return err
				}
			}
			mode := op.Mode
			l := &lock.Link{Mode: mode, Source: lib.Rel(op.Source)}
			if mode == "link" {
				if err := os.Symlink(op.Source, op.Dest); err != nil {
					if runtime.GOOS != "windows" {
						return fmt.Errorf("%s: %w", op.Dest, err)
					}
					mode = "copy"
				}
			}
			if mode == "copy" {
				if err := fsutil.CopyTree(op.Source, op.Dest); err != nil {
					return err
				}
				h, err := fsutil.HashTree(op.Dest)
				if err != nil {
					return err
				}
				l.Mode, l.Hash = "copy", h
			}
			lib.Lock.Links[key] = l
		case "remove":
			if fsutil.Lexists(op.Dest) {
				if err := os.RemoveAll(op.Dest); err != nil {
					return err
				}
			}
			delete(lib.Lock.Links, key)
		}
	}
	return nil
}

// current reports whether dest still is what the lock says we wrote.
func current(dest string, prev *lock.Link, lib *library.Library) bool {
	if prev.Mode == "link" {
		return isLinkTo(dest, lib.Abs(prev.Source))
	}
	h, err := fsutil.HashTree(dest)
	return err == nil && h == prev.Hash
}

// sameAs reports whether the recorded link already points at the wanted source.
func sameAs(dest string, prev *lock.Link, want string, lib *library.Library) bool {
	if prev.Mode == "link" {
		return isLinkTo(dest, want)
	}
	h, err := fsutil.HashTree(want)
	return err == nil && h == prev.Hash
}

func isLinkTo(dest, src string) bool {
	st, err := os.Lstat(dest)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		return false
	}
	got, err := os.Readlink(dest)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(got) {
		got = filepath.Join(filepath.Dir(dest), got)
	}
	a, _ := filepath.Abs(got)
	b, _ := filepath.Abs(src)
	return filepath.Clean(a) == filepath.Clean(b)
}

func splitKey(key string) (target, name string, ok bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			return key[:i], key[i+1:], true
		}
	}
	return "", key, false
}

// registry is set by the CLI so lock-only removals can resolve targets.
var registry *targets.Registry

// SetRegistry installs the registry used for lock-only lookups.
func SetRegistry(r *targets.Registry) { registry = r }

func regTarget(lib *library.Library, name string) (targets.Target, error) {
	if registry == nil {
		return targets.Target{}, fmt.Errorf("target %q: registry not initialised", name)
	}
	return registry.Get(name)
}
