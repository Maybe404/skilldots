package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"

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

// Unlinked are the packs (by repository) with a skill this plan leaves
// unlinked in a target it should be in: skipped because something is in
// the way or was modified, a name conflict, its source missing, or an
// unknown target.
func (p Plan) Unlinked() map[string]bool {
	out := map[string]bool{}
	for _, o := range p.Ops {
		if o.Action == "skip" && o.entry != nil && o.entry.Kind == KindVendor {
			out[o.entry.Origin] = true
		}
	}
	return out
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
	placed := loadPlaced()
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
			if e.LinkedVia != "" {
				op.Action, op.Reason = "skip", "its directory goes through "+lib.Rel(e.LinkedVia)+", a symbolic link inside the library"
				plan.Ops = append(plan.Ops, op)
				continue
			}
			if e.Missing {
				op.Action, op.Reason = "skip", "source directory missing"
				plan.Ops = append(plan.Ops, op)
				continue
			}
			if e.Refused != "" {
				op.Action, op.Reason = "skip", e.Refused
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
			case prev != nil && prev.Mode == "link" && t.Mode == "link" && isLinkTo(dest, e.Dir):
				// Already the link wanted (an app's, whose source is outside
				// the library): nothing to do.
				op.Action = "keep"
			case prev != nil && staleRoot(dest, prev, lib) != "":
				// Our link, made when the library lived somewhere else
				// (removed outright when skd's record confirms it, else
				// moved to the trash).
				op.Action, op.Reason = "replace", "points at the previous library location "+staleRoot(dest, prev, lib)
			case prev != nil && confirms(placed, dest):
				op.Action, op.Reason = "replace", "source changed"
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
			why := "unknown target " + n
			if r := registryPtr.Load(); r != nil {
				if _, held := r.WithheldTarget(n); held {
					why = "custom target " + n + " is not used (see skd targets)"
				}
			}
			plan.Ops = append(plan.Ops, Op{Action: "skip", Target: targets.Target{Name: n}, Name: e.LinkName,
				Reason: why, entry: e})
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
			// The lock comes with the library and may be forged: a record
			// of a target not in use (removed, not approved, refused, or
			// never defined) says nothing skd can trust about the place it
			// names. It is forgotten; nothing there is moved or removed,
			// with --force too.
			state := "is no longer defined"
			if r := registryPtr.Load(); r != nil {
				if _, held := r.WithheldTarget(tname); held {
					state = "is not used (see skd targets)"
				}
			}
			if LeftLink(prev, lib) {
				// What the record names is a link into the library, as skd
				// makes them: most likely left there when the target went
				// out of use. The record is kept, so every apply and `skd
				// scan` say so until the link is gone; it is still not
				// touched.
				plan.Ops = append(plan.Ops, Op{Action: "skip", Target: targets.Target{Name: tname}, Name: name, Dest: prev.Path,
					Reason: fmt.Sprintf("target %q %s; %s is a link into the library it had; skd no longer manages it: remove it yourself if it is no longer wanted", tname, state, prev.Path)})
				continue
			}
			// The record's path is not shown: it says nothing skd checked,
			// and whoever reads this (an agent too) must not take it for
			// something to remove.
			plan.Ops = append(plan.Ops, Op{Action: "forget", Target: targets.Target{Name: tname}, Name: name,
				Reason: fmt.Sprintf("target %q %s; forgetting the record (nothing on disk is touched)", tname, state)})
			continue
		}
		dest := filepath.Join(t.Path, name)
		op := Op{Target: t, Name: name, Dest: dest, Source: prev.Source, Mode: prev.Mode}
		// Nothing wants this entry any more. Only what skd can tell it
		// made goes (what clear removes outright), with --force too: the
		// record may be forged, and the entry the user's own. Nothing of
		// it is moved to the trash either: a copy a forged record's hash
		// matches, or a link a forged source makes look like skd's from
		// a previous library location, may be the user's own.
		switch {
		case !fsutil.Lexists(dest):
			op.Action, op.Reason = "remove", "already gone"
		case ownLink(dest, prev, lib) || confirms(placed, dest):
			op.Action = "remove"
		case staleDangling(dest, prev, lib) != "":
			op.Action, op.Reason = "remove", "a link to the previous library location "+staleDangling(dest, prev, lib)+", where nothing is any more"
		default:
			op.Action, op.Reason = "skip", "no longer wanted, but skd cannot tell it made it (changed since, made by an older skd, or not made by skd); left in place: it may be the user's own"
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
	// Backups lists "<dest> → <trash path>" for everything that was moved
	// into the library trash rather than removed.
	Backups []string
	// Refused lists "<dest>: <why>" for every planned change not made
	// because its target no longer leads to the directory the registry
	// found (targets.Target.Intact) or its source is reached through a
	// symbolic link inside the library; its lock record is left as it was.
	Refused []string
}

// mkdirAll is os.MkdirAll, replaceable in tests.
var mkdirAll = os.MkdirAll

// Apply executes the plan and updates the lock. The caller saves the library.
// Only what skd can tell it made is removed outright: a link into the
// library (ownLink) or what the placed file records (confirms). Anything
// else in the way, a directory, a file or a link, is moved into the
// library trash, never deleted.
func Apply(lib *library.Library, plan Plan) (*Result, error) {
	res := &Result{}
	placedMu.Lock()
	defer placedMu.Unlock()
	placed := loadPlaced()
	dirty := false
	defer func() {
		if dirty {
			savePlaced(placed)
		}
	}()
	clear := func(op Op, prev *lock.Link) error {
		if !fsutil.Lexists(op.Dest) {
			return nil
		}
		if ownLink(op.Dest, prev, lib) || confirms(placed, op.Dest) || staleDangling(op.Dest, prev, lib) != "" {
			// A link skd made holds no data, and a copy exactly as skd
			// wrote it is reproducible from the library; a link to where
			// the library was, leading nowhere now, holds only that path
			// (said in the plan). RemoveAll does not follow a link: it
			// removes the link itself.
			if err := os.RemoveAll(op.Dest); err != nil {
				return err
			}
		} else {
			to, err := Trash(lib, op.Dest)
			if err != nil {
				return fmt.Errorf("moving %s to trash: %w", op.Dest, err)
			}
			res.Backups = append(res.Backups, op.Dest+" → "+lib.Rel(to))
		}
		if _, ok := placed[placedKey(op.Dest)]; ok {
			delete(placed, placedKey(op.Dest))
			dirty = true
		}
		return nil
	}
	// intact reports whether op's target still leads where the registry
	// found it; when not, the op is refused. An earlier op of this run may
	// have changed that: a link it made in one target, or a link inside a
	// skill it linked, can lie on the way to another target. Nothing is
	// then written, moved aside or removed there.
	intact := func(op Op) bool {
		if err := op.Target.Intact(); err != nil {
			res.Refused = append(res.Refused, fmt.Sprintf("%s: not changed: %v", op.Dest, err))
			return false
		}
		return true
	}
	for _, op := range plan.Ops {
		key := op.Target.Name + "/" + op.Name
		switch op.Action {
		case "create", "replace":
			if !intact(op) {
				continue
			}
			if l := sourceLink(lib, op.Source); l != "" {
				res.Refused = append(res.Refused, fmt.Sprintf("%s: not changed: its source goes through %s, a symbolic link inside the library", op.Dest, lib.Rel(l)))
				continue
			}
			if !fsutil.Within(lib.Root, op.Source) {
				// An app's skill: checked again, as it may have changed
				// since the plan.
				if err := CheckAppDir(op.Source); err != nil {
					res.Refused = append(res.Refused, fmt.Sprintf("%s: not changed: %v", op.Dest, err))
					continue
				}
			}
			if err := mkdirAll(op.Target.Path, 0o755); err != nil {
				return res, err
			}
			if !intact(op) {
				continue
			}
			mode := op.Mode
			l := &lock.Link{Mode: mode, Source: lib.Rel(op.Source), Path: op.Dest}
			if mode == "link" && isLinkTo(op.Dest, op.Source) {
				// Already the link wanted (adopting one made by hand):
				// nothing to remove or write.
				lib.Lock.Links[key] = l
				continue
			}
			if err := clear(op, lib.Lock.Links[key]); err != nil {
				return res, err
			}
			if mode == "link" {
				if err := os.Symlink(op.Source, op.Dest); err != nil {
					if runtime.GOOS != "windows" {
						return res, fmt.Errorf("%s: %w", op.Dest, err)
					}
					mode = "copy"
				} else {
					placed[placedKey(op.Dest)] = placedEntry{Link: op.Source}
					dirty = true
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
				placed[placedKey(op.Dest)] = placedEntry{Hash: h}
				dirty = true
			}
			lib.Lock.Links[key] = l
		case "remove":
			if fsutil.Lexists(op.Dest) && !intact(op) {
				continue
			}
			if err := clear(op, lib.Lock.Links[key]); err != nil {
				return res, err
			}
			delete(lib.Lock.Links, key)
		case "forget":
			delete(lib.Lock.Links, key)
		case "keep":
			if remember(placed, op, lib) {
				dirty = true
			}
		}
	}
	return res, nil
}

// remember records in placed a link op keeps that skd made from the
// library (ownLink) but placed does not have yet (made by a version of
// skd that kept no record), and reports whether it did. Then, after the
// library moves, the link is still known as skd's. Nothing else is
// taken on: a copy's hash or an app's link may be someone's own.
func remember(placed map[string]placedEntry, op Op, lib *library.Library) bool {
	k := placedKey(op.Dest)
	if _, ok := placed[k]; ok || !ownLink(op.Dest, lib.Lock.Links[op.Target.Name+"/"+op.Name], lib) {
		return false
	}
	got, err := os.Readlink(op.Dest)
	if err != nil {
		return false
	}
	placed[k] = placedEntry{Link: got}
	return true
}

// Remember records the links a plan keeps that skd made from the library
// but has no record of (see remember). Apply does the same; this is for a
// plan with nothing to change, which is not applied.
func Remember(lib *library.Library, plan Plan) {
	if placedFile() == "" {
		return
	}
	placedMu.Lock()
	defer placedMu.Unlock()
	placed := loadPlaced()
	dirty := false
	for _, op := range plan.Ops {
		if op.Action == "keep" && remember(placed, op, lib) {
			dirty = true
		}
	}
	if dirty {
		savePlaced(placed)
	}
}

// libSource is where a recorded source is when it lies inside the
// library (as skd records the sources it links from the library), else
// "": a source outside it (an absolute path elsewhere, or a relative one
// whose ".." leads out) may name anything on the machine.
func libSource(src string, lib *library.Library) string {
	if src == "" {
		return ""
	}
	p := lib.Abs(src)
	if !fsutil.Within(lib.Root, p) {
		return ""
	}
	return p
}

// ownLink reports whether dest is a link skd made from the library: a
// symbolic link to the library-relative source prev records (libSource).
// Such a link holds no data and points into the library, so it is
// removed outright; any other link is moved to the trash.
func ownLink(dest string, prev *lock.Link, lib *library.Library) bool {
	if prev == nil || prev.Mode != "link" {
		return false
	}
	src := libSource(prev.Source, lib)
	return src != "" && isLinkTo(dest, src)
}

// LeftLink reports whether the place a lock record names (its Path) is
// a link into the library as skd makes them: a symbolic link to the
// record's source, inside the library (ownLink). For a record of a
// target no longer in use, that is a link it left behind; skd reads
// that path, never writes it.
func LeftLink(prev *lock.Link, lib *library.Library) bool {
	return prev != nil && filepath.IsAbs(prev.Path) && ownLink(prev.Path, prev, lib)
}

// sourceLink returns the first symbolic link on the way from the library
// root down to src (a source inside the library), or "": a link there
// would make the skill's files whatever it points at, outside the
// library. The library root itself may be reached through links. A
// source outside the library (an app's) is not checked.
func sourceLink(lib *library.Library, src string) string {
	if !fsutil.Within(lib.Root, src) {
		return ""
	}
	return fsutil.LinkOnPath(lib.Root, src)
}

// current reports whether dest still is what the lock says we wrote.
// A link counts only when its recorded source is inside the library
// (libSource); a copy only when dest is a directory with that hash, which
// a forged record can match, so a copy that is current is still moved to
// the trash, not deleted, unless skd's own record confirms it (Apply).
func current(dest string, prev *lock.Link, lib *library.Library) bool {
	if prev.Mode == "link" {
		return ownLink(dest, prev, lib)
	}
	if st, err := os.Lstat(dest); err != nil || !st.IsDir() {
		return false
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

// staleRoot returns the old library root when dest is a symlink skilldots
// recorded, pointing at the recorded library-relative source under a root
// other than the current one (the library was moved or copied). It returns
// "" otherwise.
func staleRoot(dest string, prev *lock.Link, lib *library.Library) string {
	if prev.Mode != "link" || prev.Source == "" || filepath.IsAbs(prev.Source) {
		return ""
	}
	got, err := os.Readlink(dest)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(got) {
		got = filepath.Join(filepath.Dir(dest), got)
	}
	got = filepath.Clean(got)
	suffix := string(filepath.Separator) + filepath.FromSlash(prev.Source)
	if !strings.HasSuffix(got, suffix) {
		return ""
	}
	old := strings.TrimSuffix(got, suffix)
	if old == "" || old == filepath.Clean(lib.Root) {
		return ""
	}
	if a, err := filepath.EvalSymlinks(old); err == nil {
		if b, err := filepath.EvalSymlinks(lib.Root); err == nil && a == b {
			return ""
		}
	}
	return old
}

// staleDangling returns the previous library root when dest is a link
// skd made there before the library moved (staleRoot) that now leads
// nowhere, else "". It must be exactly what skd writes: an absolute link
// to the old root joined with the recorded source, a clean path inside
// the library under skills/, private/ or vendor/. Such a link holds no
// data, only a path the plan shows, so it is removed outright even when
// skd's placed record does not know it (made before there was one).
//
// A forged record can claim a link of the user's only when that link,
// in an agent's directory under the record's name, is absolute, ends in
// /skills/<name>, /private/<name> or /vendor/.../<name>, and leads
// nowhere: losing it loses no data, and the path is in the output.
func staleDangling(dest string, prev *lock.Link, lib *library.Library) string {
	if prev == nil || prev.Mode != "link" || libSource(prev.Source, lib) == "" {
		return ""
	}
	if top, _, _ := strings.Cut(filepath.ToSlash(prev.Source), "/"); top != library.DirSkills && top != library.DirPrivate && top != library.DirVendor {
		return ""
	}
	got, err := os.Readlink(dest)
	if err != nil || !filepath.IsAbs(got) {
		return ""
	}
	old := staleRoot(dest, prev, lib)
	if old == "" {
		return ""
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		return ""
	}
	return old
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

// registryPtr is set by the CLI so lock-only removals can resolve targets.
// It is atomic: the dashboard opens the library from two goroutines when
// it checks upstream while an operation runs.
var registryPtr atomic.Pointer[targets.Registry]

// SetRegistry installs the registry used for lock-only lookups.
func SetRegistry(r *targets.Registry) { registryPtr.Store(r) }

func evalClean(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func regTarget(lib *library.Library, name string) (targets.Target, error) {
	registry := registryPtr.Load()
	if registry == nil {
		return targets.Target{}, fmt.Errorf("target %q: registry not initialised", name)
	}
	return registry.Get(name)
}
