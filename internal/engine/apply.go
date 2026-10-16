package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

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

// BackupDir is the folder inside a target where displaced files are kept.
const BackupDir = ".skilldots-backup"

// Result reports what Apply did beyond the plan.
type Result struct {
	// Backups lists "<dest> → <backup path>" for every real directory that
	// was moved aside rather than deleted.
	Backups []string
}

// Apply executes the plan and updates the lock. The caller saves the library.
// A symlink in the way is simply replaced; anything else (a real directory,
// or a copy modified since skilldots wrote it) is moved into the target's
// BackupDir first, never deleted.
func Apply(lib *library.Library, plan Plan) (*Result, error) {
	res := &Result{}
	stamp := time.Now().Format("20060102-150405")
	clear := func(op Op, prev *lock.Link) error {
		if !fsutil.Lexists(op.Dest) {
			return nil
		}
		st, err := os.Lstat(op.Dest)
		if err != nil {
			return err
		}
		disposable := st.Mode()&os.ModeSymlink != 0 || (prev != nil && current(op.Dest, prev, lib))
		if disposable {
			return os.RemoveAll(op.Dest)
		}
		bdir := filepath.Join(op.Target.Path, BackupDir, stamp)
		if err := os.MkdirAll(bdir, 0o755); err != nil {
			return err
		}
		to := filepath.Join(bdir, op.Name)
		if err := os.Rename(op.Dest, to); err != nil {
			return fmt.Errorf("backing up %s: %w", op.Dest, err)
		}
		res.Backups = append(res.Backups, op.Dest+" → "+to)
		return nil
	}
	for _, op := range plan.Ops {
		key := op.Target.Name + "/" + op.Name
		switch op.Action {
		case "create", "replace":
			if err := os.MkdirAll(op.Target.Path, 0o755); err != nil {
				return res, err
			}
			if err := clear(op, lib.Lock.Links[key]); err != nil {
				return res, err
			}
			mode := op.Mode
			l := &lock.Link{Mode: mode, Source: lib.Rel(op.Source)}
			if mode == "link" {
				if err := os.Symlink(op.Source, op.Dest); err != nil {
					if runtime.GOOS != "windows" {
						return res, fmt.Errorf("%s: %w", op.Dest, err)
					}
					mode = "copy"
				}
			}
			if mode == "copy" {
				if err := fsutil.CopyTree(op.Source, op.Dest); err != nil {
					return res, err
				}
				h, err := fsutil.HashTree(op.Dest)
				if err != nil {
					return res, err
				}
				l.Mode, l.Hash = "copy", h
			}
			lib.Lock.Links[key] = l
		case "remove":
			if err := clear(op, lib.Lock.Links[key]); err != nil {
				return res, err
			}
			delete(lib.Lock.Links, key)
		}
	}
	return res, nil
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
