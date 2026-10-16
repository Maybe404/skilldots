package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/safetext"
	"github.com/Maybe404/skilldots/internal/targets"
)

// TrashDir is the library folder where skilldots parks anything it takes
// out of the way. Nothing is ever deleted outright; `skd trash empty` is
// the only destructive step and it is reserved for a person at a terminal.
const TrashDir = ".trash"

// TrashEntry is one parked directory.
type TrashEntry struct {
	ID     string    `json:"id"` // "<batch>/<name>"
	Name   string    `json:"name"`
	Origin string    `json:"origin"` // where it came from
	Path   string    `json:"path"`   // where it is now
	When   time.Time `json:"when"`
	// Restorable is false when `skd trash restore` refuses the entry;
	// Problem then says why (it holds no control characters).
	Restorable bool   `json:"restorable"`
	Problem    string `json:"problem,omitempty"`
}

// trashBatch is the per-run folder so one command's moves stay together.
var trashBatch string

func batch() string {
	if trashBatch == "" {
		trashBatch = time.Now().Format("20060102-150405")
	}
	return trashBatch
}

// Trash moves src into the library trash and records its origin. A
// symbolic link is moved as it is (the link, not what it points at): it
// may be someone's own, and only links skd can tell it made are removed
// outright (Apply). `skd trash` lists such an entry, but restore leaves
// it to a person (see ListTrash).
func Trash(lib *library.Library, src string) (string, error) {
	if _, err := os.Lstat(src); err != nil {
		return "", err
	}
	dir := filepath.Join(lib.Root, TrashDir, batch())
	// The trash must be the library's own directory: a cloned library's
	// .trash can be a link to anywhere (a directory that is cleaned up,
	// or shared), and what is moved there would go with it.
	if l := fsutil.LinkOnPath(lib.Root, dir); l != "" {
		return "", fmt.Errorf("the trash %s is reached through the symbolic link %s; skd moves nothing there (make it a directory of its own)", dir, l)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// The record is absolute: a library or a target named by a relative
	// path would otherwise be restored relative to wherever skd runs.
	if abs, err := filepath.Abs(src); err == nil {
		src = abs
	}
	name := filepath.Base(src)
	// Neither the entry nor its record may be there already, in any
	// form: a batch directory of a cloned library's trash is someone
	// else's to fill, and a "<name>.origin" there that is a link to a
	// file of the user's would have the record written over that file.
	dest := filepath.Join(dir, name)
	for i := 2; fsutil.Lexists(dest) || fsutil.Lexists(dest+".origin"); i++ {
		dest = filepath.Join(dir, fmt.Sprintf("%s-%d", name, i))
	}
	if err := move(src, dest); err != nil {
		return "", err
	}
	// The record is created only as a new file: O_EXCL does not follow a
	// link, so one put there since the check above is an error, never a
	// write through it.
	f, err := os.OpenFile(dest+".origin", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		_, err = f.WriteString(src + "\n")
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		return "", fmt.Errorf("%s was moved to %s, but where it came from could not be recorded: %w", src, dest, err)
	}
	return dest, nil
}

// rename is os.Rename, replaceable in tests.
var rename = os.Rename

// move renames src to dst. Only when that fails because they are on
// different devices does it fall back to an exact copy (everything,
// including .git, node_modules and symlinks) followed by removing src.
// Any other failure is returned with src untouched.
func move(src, dst string) error {
	err := rename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := fsutil.CopyAll(src, dst); err != nil {
		os.RemoveAll(dst) // a partial copy; src is still complete
		return fmt.Errorf("moving %s across devices: %w", src, err)
	}
	return os.RemoveAll(src)
}

// ListTrash returns every parked directory, newest first, each marked
// with whether Restore would put it back (reg is the target registry;
// nil allows only the library's vendor/).
func ListTrash(lib *library.Library, reg *targets.Registry) ([]TrashEntry, error) {
	root := filepath.Join(lib.Root, TrashDir)
	if err := realDir(root); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("the trash %q: %w; skd does not follow it", root, err)
	}
	batches, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []TrashEntry
	for _, b := range batches {
		// DirEntry types come from the directory itself: a symbolic link
		// to a directory is not a directory here, and is left out.
		if !b.IsDir() {
			continue
		}
		items, _ := os.ReadDir(filepath.Join(root, b.Name()))
		for _, it := range items {
			link := it.Type()&os.ModeSymlink != 0
			if !it.IsDir() && !(link && fsutil.Lexists(filepath.Join(root, b.Name(), it.Name())+".origin")) {
				continue
			}
			p := filepath.Join(root, b.Name(), it.Name())
			when, _ := time.ParseInLocation("20060102-150405", b.Name(), time.Local)
			e := TrashEntry{ID: b.Name() + "/" + it.Name(), Name: it.Name(), Path: p, When: when}
			origin, err := readOrigin(p)
			if err == nil {
				e.Origin = origin
				err = checkOrigin(lib, reg, origin)
			}
			if link {
				// A link moved aside (apply --force, adopt). Restoring one
				// would put a link to anywhere into an agent's directory,
				// and a cloned library's trash may hold any: a person
				// moves it back.
				to, _ := os.Readlink(p)
				err = fmt.Errorf("it is a symbolic link (to %q); skd does not restore links: whether to put it back is the user's decision", to)
			}
			e.Restorable = err == nil
			if err != nil {
				e.Problem = err.Error()
			}
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// Restore moves a parked directory back to its origin. Only an entry skd
// could have made is restored: id names a directory exactly two levels
// down in the library's .trash (no "..", no separator, nothing reached
// through a symbolic link), and its recorded origin is a place skd moves
// directories from (checkOrigin). The trash of a library cloned from
// someone else is theirs to write; without these checks restoring an
// entry would move any directory to any place.
func Restore(lib *library.Library, reg *targets.Registry, id string) (string, error) {
	p, err := entryPath(lib, id)
	if err != nil {
		return "", err
	}
	dest, err := readOrigin(p)
	if err == nil {
		err = checkOrigin(lib, reg, dest)
	}
	if err != nil {
		return "", fmt.Errorf("trash entry %q cannot be restored: %w; restore it by hand from %q", id, err, p)
	}
	if fsutil.Lexists(dest) {
		return "", fmt.Errorf("%q already exists; remove it first (skd disable/remove) or restore by hand from %q", dest, p)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := move(p, dest); err != nil {
		return "", err
	}
	os.Remove(p + ".origin")
	pruneBatch(filepath.Dir(p))
	return dest, nil
}

// entryPath is where the trash entry id is, after checking that id is
// "<batch>/<name>" and that the trash, the batch and the entry are
// directories of their own, none a symbolic link.
func entryPath(lib *library.Library, id string) (string, error) {
	parts := strings.Split(id, "/")
	bad := len(parts) != 2
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `/\`+"\x00") || filepath.Base(part) != part {
			bad = true
		}
	}
	if bad {
		return "", fmt.Errorf("no trash entry %q (an id is <batch>/<name>, as `skd trash` lists it)", id)
	}
	root := filepath.Join(lib.Root, TrashDir)
	p := root
	for _, d := range []string{"", parts[0], parts[1]} {
		p = filepath.Join(p, d)
		if err := realDir(p); os.IsNotExist(err) {
			return "", fmt.Errorf("no trash entry %q", id)
		} else if err != nil {
			return "", fmt.Errorf("trash entry %q: %q %w", id, p, err)
		}
	}
	return p, nil
}

// realDir reports whether p is a directory itself: an error for a
// symbolic link (even to a directory) or anything else.
func realDir(p string) error {
	st, err := os.Lstat(p)
	switch {
	case err != nil:
		return err
	case st.Mode()&os.ModeSymlink != 0:
		return errors.New("is a symbolic link")
	case !st.IsDir():
		return errors.New("is not a directory")
	}
	return nil
}

// maxOrigin bounds the record read: skd writes one path and a newline.
const maxOrigin = 16 << 10

// readOrigin reads the record next to the entry at p: a regular file (a
// FIFO or a link to /dev/zero would hang or never end) holding one path.
func readOrigin(p string) (string, error) {
	f := p + ".origin"
	st, err := os.Lstat(f)
	if err != nil {
		return "", errors.New("where it came from is not recorded")
	}
	if !st.Mode().IsRegular() || st.Size() > maxOrigin {
		return "", fmt.Errorf("its record %q is not a file skd wrote", filepath.Base(f))
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return "", err
	}
	s := strings.TrimSuffix(string(b), "\n")
	return strings.TrimSuffix(s, "\r"), nil // a checkout may add a CR
}

// checkOrigin reports why origin is not a place Restore puts a directory
// back. skd moves directories into the trash from two places only, and
// those are the only places it restores to:
//
//   - an entry of an agent's skill directory (a target in reg: apply
//     --force, disable, remove and adopt move what is in the way or
//     adopted there), that is a direct child of the target's directory;
//     for a custom target (reg has one only when a person approved it on
//     this machine) also named like a skill (manifest.CheckName: not
//     ".ssh"), as apply only ever writes such names there;
//   - inside the library's vendor/ (update and remove move vendored
//     skills, purge a whole pack).
//
// The record must be an absolute, clean path (no "..", ".", doubled or
// trailing separators) without control characters, as skd writes it.
// Symbolic links on the way are resolved before comparing, so neither a
// link inside vendor/ nor a link elsewhere can lead out of these places.
func checkOrigin(lib *library.Library, reg *targets.Registry, origin string) error {
	switch {
	case safetext.Has(origin):
		return fmt.Errorf("where it came from (%q) has control characters", origin)
	case !filepath.IsAbs(origin):
		return fmt.Errorf("where it came from (%q) is not an absolute path", origin)
	case filepath.Clean(origin) != origin:
		return fmt.Errorf("where it came from (%q) is not a clean path", origin)
	}
	parent, base := filepath.Dir(origin), filepath.Base(origin)
	if base == "." || base == ".." || parent == origin {
		return fmt.Errorf("where it came from (%q) is not a directory skd manages", origin)
	}
	at := resolve(parent)
	if reg != nil {
		var custom []string
		for _, t := range reg.All() {
			// A target that no longer leads where it did when the registry
			// was built (a link on the way changed) is no place to restore to.
			if t.Intact() != nil {
				continue
			}
			if tp, err := filepath.Abs(t.Path); err == nil && (parent == tp || fsutil.SameDir(at, resolve(tp), false)) {
				if t.Builtin {
					return nil
				}
				custom = append(custom, t.Name)
			}
		}
		// A custom target (in reg only when approved here) gets back only
		// what apply puts there: an entry named like a skill. Not ".ssh"
		// or anything else starting with a dot.
		if len(custom) > 0 {
			if err := manifest.CheckName(base); err != nil {
				return fmt.Errorf("where it came from (%q) is in the custom target %s, and %q is not a skill's name", origin, custom[0], base)
			}
			return nil
		}
	}
	// vendor/ itself must be the library's own directory: a link there
	// would make anywhere it points "inside vendor/". The comparison is by
	// file identity, so another spelling of the library's path (case, or
	// Unicode normalisation, on a file system that ignores them, as macOS
	// does) is still the library.
	if vd, err := filepath.Abs(lib.VendorDir()); err == nil && insideStrict(resolve(vd), filepath.Join(at, base)) {
		if err := realDir(vd); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("the library's vendor/ %w", err)
		}
		return nil
	}
	return fmt.Errorf("where it came from (%q) is neither in an agent's skill directory nor in the library's vendor/", origin)
}

// insideStrict reports whether p is inside dir and not dir itself,
// comparing by file identity (fsutil.Inside).
func insideStrict(dir, p string) bool {
	return !fsutil.SameDir(p, dir, false) && fsutil.Inside(dir, p, false)
}

// resolve is fsutil.Resolve.
func resolve(p string) string { return fsutil.Resolve(p) }

// EmptyTrash deletes everything parked. Irreversible.
func EmptyTrash(lib *library.Library) (int, error) {
	entries, err := ListTrash(lib, nil)
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(filepath.Join(lib.Root, TrashDir)); err != nil {
		return 0, err
	}
	return len(entries), nil
}

func pruneBatch(dir string) {
	if items, err := os.ReadDir(dir); err == nil && len(items) == 0 {
		os.Remove(dir)
	}
}
