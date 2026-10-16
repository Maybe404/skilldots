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
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/targets"
)

// Op is one planned file-system change.
type Op struct {
	// Action: create, replace, remove, forget (drop a lock record only),
	// skip, keep.
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

// Change is the compact, machine-readable form of an Op.
type Change struct {
	Action string `json:"action"`
	Target string `json:"target"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Reason string `json:"reason,omitempty"`
}

// Changes lists the ops that do something (everything except "keep").
func (p Plan) ChangeList() []Change {
	out := []Change{}
	for _, o := range p.Ops {
		if o.Action == "keep" {
			continue
		}
		out = append(out, Change{o.Action, o.Target.Name, o.Name, o.Dest, o.Reason})
	}
	return out
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
		if o.Action == "create" || o.Action == "replace" || o.Action == "remove" || o.Action == "forget" {
			return true
		}
	}
	return false
}

// MakePlan compares entries against the lock and disk.
func MakePlan(lib *library.Library, entries []Entry, force bool) Plan {
	return MakePlanForce(lib, entries, func(string) bool { return force })
}

// MakePlanForce is MakePlan with force decided per "<target>/<name>" key,
// so a caller can force only the entries it is responsible for.
func MakePlanForce(lib *library.Library, entries []Entry, forceKey func(key string) bool) Plan {
	var plan Plan
	desired := map[string]bool{}
	conflictShown := map[string]bool{}
	for i := range entries {
		e := &entries[i]
		for _, t := range e.Targets {
			dest := filepath.Join(t.Path, e.LinkName)
			key := t.Name + "/" + e.LinkName
			desired[key] = true
			op := Op{Target: t, Name: e.LinkName, Dest: dest, Source: e.Dir, Mode: t.Mode, entry: e}
			if e.Conflict != "" {
				if !conflictShown[key] {
					conflictShown[key] = true
					op.Action, op.Reason = "skip", "name conflict: "+e.Conflict
					plan.Ops = append(plan.Ops, op)
				}
				continue
			}
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
				if forceKey(key) {
					op.Action, op.Reason = "replace", "modified since skilldots wrote it (--force)"
				} else {
					op.Action, op.Reason = "skip", "modified since skilldots wrote it; use --force"
				}
			default:
				// Exists and we never placed it.
				if isLinkTo(dest, e.Dir) {
					op.Action, op.Reason = "create", "adopting existing link"
				} else if forceKey(key) {
					op.Action, op.Reason = "replace", "unmanaged entry (--force)"
				} else {
					op.Action, op.Reason = "skip", "exists and not managed by skilldots; use --force"
				}
			}
			plan.Ops = append(plan.Ops, op)
		}
		for _, n := range e.UnknownTargets {
			// Leave whatever is recorded for this target alone.
			desired[n+"/"+e.LinkName] = true
			plan.Ops = append(plan.Ops, Op{Action: "skip", Target: targets.Target{Name: n}, Name: e.LinkName,
				Reason: "unknown target " + n})
		}
	}
	// Anything in the lock no longer desired gets removed.
	for _, key := range lib.Lock.LinkKeys() {
		if desired[key] {
			continue
		}
		prev := lib.Lock.Links[key]
		tname, name, _ := splitKey(key)
		if err := manifest.CheckName(name); err != nil {
			plan.Ops = append(plan.Ops, Op{Action: "skip", Target: targets.Target{Name: tname}, Name: name,
				Reason: "skills.lock link record: " + err.Error()})
			continue
		}
		t, err := regTarget(lib, tname)
		if err != nil {
			if prev.Path == "" || filepath.Base(prev.Path) != name {
				// Nowhere to look: stop tracking it rather than nag forever.
				plan.Ops = append(plan.Ops, Op{Action: "forget", Target: targets.Target{Name: tname}, Name: name,
					Reason: fmt.Sprintf("target %q is no longer defined and its location was not recorded; forgetting the record (nothing on disk is touched)", tname)})
				continue
			}
			t = targets.Target{Name: tname, Path: filepath.Dir(prev.Path), Mode: prev.Mode}
		}
		dest := filepath.Join(t.Path, name)
		op := Op{Target: t, Name: name, Dest: dest, Source: prev.Source, Mode: prev.Mode}
		switch {
		case !fsutil.Lexists(dest):
			op.Action, op.Reason = "remove", "already gone"
		case current(dest, prev, lib) || forceKey(key):
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

// Result reports what Apply did beyond the plan.
type Result struct {
	// Backups lists "<dest> → <trash path>" for every real directory that
	// was moved into the library trash rather than deleted.
	Backups []string
}

// Apply executes the plan and updates the lock. The caller saves the library.
// A symlink in the way is simply replaced; anything else (a real directory,
// or a copy modified since skilldots wrote it) is moved into the library
// trash first, never deleted.
func Apply(lib *library.Library, plan Plan) (*Result, error) {
	res := &Result{}
	clear := func(op Op, prev *lock.Link) error {
		if !fsutil.Lexists(op.Dest) {
			return nil
		}
		st, err := os.Lstat(op.Dest)
		if err != nil {
			return err
		}
		// A symlink holds no data. A copy is only discarded when it is
		// exactly what skilldots wrote: nothing added that the hash does
		// not see (.git, node_modules, symlinks). Anything else is trashed.
		if st.Mode()&os.ModeSymlink != 0 {
			return os.Remove(op.Dest)
		}
		if prev != nil && prev.Mode == "copy" && fsutil.Pristine(op.Dest, prev.Hash) {
			return os.RemoveAll(op.Dest)
		}
		to, err := Trash(lib, op.Dest)
		if err != nil {
			return fmt.Errorf("moving %s to trash: %w", op.Dest, err)
		}
		res.Backups = append(res.Backups, op.Dest+" → "+lib.Rel(to))
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
			l := &lock.Link{Mode: mode, Source: lib.Rel(op.Source), Path: op.Dest}
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
		case "forget":
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
