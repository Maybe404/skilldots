package app

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Maybe404/skilldots/internal/config"
	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/manifest"
)

/*
 * Zip backups: the library in one file, for people who keep no git remote.
 *
 * A backup holds the whole library as it is on disk (skills.toml,
 * skills.lock, skills.history.jsonl, vendor/, skills/, private/ and both git
 * repositories with their history) under library/, and a manifest,
 * skilldots-backup.json, that lists every file with its size and SHA-256.
 * Left out: the library trash, the temporary files of an interrupted save,
 * and skills that live outside the library ([[app]] entries: the skilldots
 * skill ships with the app and is updated with it).
 *
 * Restore checks the whole archive against the manifest before writing
 * anything, unpacks it next to the destination and renames it into place,
 * so a library is either restored whole or not at all. It never writes into
 * a directory that has something in it.
 */

const (
	// BackupManifest is the manifest's name at the top of a backup.
	BackupManifest = "skilldots-backup.json"
	// backupFormat is the manifest's format; restore refuses others.
	backupFormat = 1
	// backupRoot is the directory of the library inside a backup.
	backupRoot = "library"
)

// BackupFile is one file, directory or symbolic link of a backup.
type BackupFile struct {
	Path string `json:"path"`
	// Type is "file", "dir" or "symlink".
	Type   string `json:"type"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	// Target is a symbolic link's target.
	Target string `json:"target,omitempty"`
}

// BackupSkill names one skill a backup holds.
type BackupSkill struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Origin string `json:"origin,omitempty"`
}

// BackupInfo is the manifest of a backup.
type BackupInfo struct {
	Format  int           `json:"format"`
	Version string        `json:"version"`
	Created time.Time     `json:"created"`
	Library string        `json:"library"`
	Skills  []BackupSkill `json:"skills"`
	Files   []BackupFile  `json:"files"`
}

// BackupResult is what Backup made.
type BackupResult struct {
	Path   string        `json:"path"`
	Skills []BackupSkill `json:"skills"`
	Files  int           `json:"files"`
	Bytes  int64         `json:"bytes"`
}

// BackupName is the file name of a backup made at t.
func BackupName(t time.Time) string {
	return "skilldots-backup-" + t.Format("20060102-150405") + ".zip"
}

// BackupZip writes a zip backup of the library into dir (created if needed)
// and returns its path. version is the skd version recorded in it. An
// existing file is never overwritten.
func (s *Session) BackupZip(dir, version string) (*BackupResult, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	root := s.Lib.Root
	if fsutil.Inside(fsutil.Resolve(root), fsutil.Resolve(dir), true) {
		return nil, i18n.Errorf("%s is inside the library; choose a directory outside it for the backup", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := s.Resolve()
	if err != nil {
		return nil, err
	}
	info := &BackupInfo{Format: backupFormat, Version: version, Created: time.Now().UTC().Truncate(time.Second), Library: root, Skills: []BackupSkill{}}
	for _, e := range entries {
		if e.Kind == engine.KindApp {
			continue
		}
		info.Skills = append(info.Skills, BackupSkill{Name: e.LinkName, Kind: string(e.Kind), Origin: e.Origin})
	}
	files, err := backupFiles(root, s.appDirs(entries))
	if err != nil {
		return nil, err
	}
	info.Files = files

	name := filepath.Join(dir, BackupName(time.Now()))
	if fsutil.Lexists(name) {
		return nil, i18n.Errorf("%s already exists", name)
	}
	tmp, err := os.CreateTemp(dir, ".skilldots-backup-*.tmp")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if err := writeBackup(tmp, root, info); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	// A hard link refuses to replace a file that appeared meanwhile. Some
	// disks (exFAT, FAT) have none: check, then rename.
	if err := os.Link(tmp.Name(), name); err != nil {
		if os.IsExist(err) || fsutil.Lexists(name) {
			return nil, i18n.Errorf("%s already exists", name)
		}
		if err := os.Rename(tmp.Name(), name); err != nil {
			return nil, err
		}
	}
	st, _ := os.Stat(name)
	res := &BackupResult{Path: name, Skills: info.Skills, Files: len(files)}
	if st != nil {
		res.Bytes = st.Size()
	}
	return res, nil
}

// appDirs are the directories of [[app]] skills that sit inside the
// library: they are not backed up.
func (s *Session) appDirs(entries []engine.Entry) []string {
	var out []string
	for _, e := range entries {
		if e.Kind == engine.KindApp && e.Dir != "" {
			out = append(out, e.Dir)
		}
	}
	return out
}

// skipBackup reports whether rel (slash-separated, relative to the
// library) stays out of a backup.
func skipBackup(rel string, isDir bool) bool {
	if rel == engine.TrashDir {
		return true
	}
	base := path.Base(rel)
	return !isDir && strings.HasPrefix(base, ".") && strings.Contains(base, ".tmp-")
}

// backupFiles lists what a backup of root holds. The top-level skills/,
// vendor/ and private/ are followed when they are links to directories
// elsewhere, so their skills are in the backup; any other link is kept as a
// link.
func backupFiles(root string, skip []string) ([]BackupFile, error) {
	var out []BackupFile
	var walk func(dir, rel string) error
	walk = func(dir, rel string) error {
		list, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, d := range list {
			p := filepath.Join(dir, d.Name())
			r := path.Join(rel, d.Name())
			fi, err := os.Lstat(p)
			if err != nil {
				return err
			}
			if slices.ContainsFunc(skip, func(s string) bool { return fsutil.SameDir(s, p, false) }) {
				continue
			}
			if fi.Mode()&os.ModeSymlink != 0 && rel == "" && slices.Contains([]string{library.DirSkills, library.DirVendor, library.DirPrivate}, d.Name()) {
				if st, err := os.Stat(p); err == nil && st.IsDir() {
					fi = st
				}
			}
			switch {
			case fi.IsDir():
				if skipBackup(r, true) {
					continue
				}
				out = append(out, BackupFile{Path: r, Type: "dir", Mode: uint32(fi.Mode().Perm())})
				if err := walk(p, r); err != nil {
					return err
				}
			case fi.Mode()&os.ModeSymlink != 0:
				t, err := os.Readlink(p)
				if err != nil {
					return err
				}
				out = append(out, BackupFile{Path: r, Type: "symlink", Mode: 0o777, Target: filepath.ToSlash(t)})
			case fi.Mode().IsRegular():
				if skipBackup(r, false) {
					continue
				}
				sum, err := hashFile(p)
				if err != nil {
					return err
				}
				out = append(out, BackupFile{Path: r, Type: "file", Mode: uint32(fi.Mode().Perm()), Size: fi.Size(), SHA256: sum})
			}
			// Sockets, devices and pipes are not library content.
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(out, func(f BackupFile) bool { return f.Path == manifest.FileName }) {
		return nil, i18n.Errorf("no %s in %s", manifest.FileName, root)
	}
	return out, nil
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// realPath is where a backup file of root is read from (through the
// top-level links backupFiles followed).
func realPath(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

func writeBackup(w io.Writer, root string, info *BackupInfo) error {
	z := zip.NewWriter(w)
	m, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	hw, err := z.CreateHeader(&zip.FileHeader{Name: BackupManifest, Method: zip.Deflate, Modified: info.Created})
	if err != nil {
		return err
	}
	if _, err := hw.Write(m); err != nil {
		return err
	}
	for _, f := range info.Files {
		h := &zip.FileHeader{Name: backupRoot + "/" + f.Path, Method: zip.Deflate, Modified: info.Created}
		switch f.Type {
		case "dir":
			h.Name += "/"
			h.Method = zip.Store
			h.SetMode(fs.ModeDir | fs.FileMode(f.Mode))
			if _, err := z.CreateHeader(h); err != nil {
				return err
			}
		case "symlink":
			h.Method = zip.Store
			h.SetMode(fs.ModeSymlink | 0o777)
			hw, err := z.CreateHeader(h)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(hw, f.Target); err != nil {
				return err
			}
		default:
			h.SetMode(fs.FileMode(f.Mode))
			hw, err := z.CreateHeader(h)
			if err != nil {
				return err
			}
			src, err := os.Open(realPath(root, f.Path))
			if err != nil {
				return err
			}
			hsh := sha256.New()
			n, err := io.Copy(io.MultiWriter(hw, hsh), src)
			src.Close()
			if err != nil {
				return err
			}
			// The file changed while the backup was made.
			if n != f.Size || hex.EncodeToString(hsh.Sum(nil)) != f.SHA256 {
				return i18n.Errorf("%s changed while the backup was being made; try again", f.Path)
			}
		}
	}
	return z.Close()
}

// ErrBadBackup marks a backup that is damaged or not a skilldots backup.
var ErrBadBackup = i18n.New("not a usable skilldots backup")

// ReadBackup opens the backup at p and checks every file in it against its
// manifest. Nothing is written.
func ReadBackup(p string) (*BackupInfo, error) {
	z, err := zip.OpenReader(p)
	if err != nil {
		return nil, i18n.Errorf("%s is %w: %v", p, ErrBadBackup, err)
	}
	defer z.Close()
	info, _, err := checkBackup(&z.Reader, p)
	return info, err
}

// checkBackup reads the manifest of z and verifies every entry. It returns
// the manifest and the archive's files by their library path.
func checkBackup(z *zip.Reader, name string) (*BackupInfo, map[string]*zip.File, error) {
	bad := func(why string) error {
		return i18n.Errorf("%s is %w: %s", name, ErrBadBackup, why)
	}
	var mf *zip.File
	byPath := map[string]*zip.File{}
	for _, f := range z.File {
		if f.Name == BackupManifest {
			mf = f
			continue
		}
		rel, ok := strings.CutPrefix(f.Name, backupRoot+"/")
		rel = strings.TrimSuffix(rel, "/")
		if !ok || !safeRel(rel) {
			return nil, nil, bad(i18n.Sprintf("unexpected entry %q", f.Name))
		}
		if byPath[rel] != nil {
			return nil, nil, bad(i18n.Sprintf("%q appears twice", rel))
		}
		byPath[rel] = f
	}
	if mf == nil {
		return nil, nil, bad(i18n.Sprintf("no %s in it", BackupManifest))
	}
	r, err := mf.Open()
	if err != nil {
		return nil, nil, bad(i18n.Sprintf("%v", err))
	}
	var info BackupInfo
	err = json.NewDecoder(io.LimitReader(r, 64<<20)).Decode(&info)
	r.Close()
	if err != nil {
		return nil, nil, bad(i18n.Sprintf("cannot read %s: %v", BackupManifest, err))
	}
	if info.Format != backupFormat {
		return nil, nil, bad(i18n.Sprintf("format %d; this skd reads format %d (a newer skd made it?)", info.Format, backupFormat))
	}
	listed := map[string]bool{}
	hasManifest := false
	for _, f := range info.Files {
		if !safeRel(f.Path) || listed[f.Path] {
			return nil, nil, bad(i18n.Sprintf("bad file %q in %s", f.Path, BackupManifest))
		}
		listed[f.Path] = true
		hasManifest = hasManifest || (f.Path == manifest.FileName && f.Type == "file")
		zf := byPath[f.Path]
		if zf == nil {
			return nil, nil, bad(i18n.Sprintf("%s is missing", f.Path))
		}
		switch f.Type {
		case "dir":
			if !zf.FileInfo().IsDir() {
				return nil, nil, bad(i18n.Sprintf("%s should be a directory", f.Path))
			}
		case "symlink":
			if zf.Mode()&fs.ModeSymlink == 0 {
				return nil, nil, bad(i18n.Sprintf("%s should be a symbolic link", f.Path))
			}
			b, err := readAll(zf, 4096)
			if err != nil || string(b) != f.Target {
				return nil, nil, bad(i18n.Sprintf("%s does not match its checksum", f.Path))
			}
		case "file":
			if !zf.Mode().IsRegular() {
				return nil, nil, bad(i18n.Sprintf("%s should be a file", f.Path))
			}
			if err := verifyEntry(zf, f, io.Discard); err != nil {
				return nil, nil, bad(i18n.Sprintf("%s %v", f.Path, err))
			}
		default:
			return nil, nil, bad(i18n.Sprintf("unknown type %q of %s", f.Type, f.Path))
		}
	}
	for rel := range byPath {
		if !listed[rel] {
			return nil, nil, bad(i18n.Sprintf("%s is not in %s", rel, BackupManifest))
		}
	}
	if !hasManifest {
		return nil, nil, bad(i18n.Sprintf("no %s in it", manifest.FileName))
	}
	return &info, byPath, nil
}

func readAll(f *zip.File, limit int64) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, limit))
}

// verifyEntry copies f to w and checks its size and checksum.
func verifyEntry(zf *zip.File, f BackupFile, w io.Writer) error {
	r, err := zf.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(r, f.Size+1))
	if err != nil {
		return err
	}
	if n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		return i18n.New("does not match its checksum")
	}
	return nil
}

// safeRel reports whether rel is a clean relative path inside the library.
func safeRel(rel string) bool {
	if rel == "" || strings.Contains(rel, "\\") || path.IsAbs(rel) || path.Clean(rel) != rel || rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}
	return !strings.ContainsRune(rel, 0)
}

// RestoreResult is what RestoreBackup did.
type RestoreResult struct {
	Library string      `json:"library"`
	Info    *BackupInfo `json:"backup"`
	// Skipped are symbolic links not restored: their target is absolute or
	// outside the library.
	Skipped []string `json:"skipped,omitempty"`
}

// RestoreBackup checks the backup at p and unpacks its library into dest,
// which must not exist or be an empty directory, and records dest as the
// current library. It links nothing: open the library and apply.
func RestoreBackup(p, dest string) (*RestoreResult, error) {
	dest, err := filepath.Abs(dest)
	if err != nil {
		return nil, err
	}
	if err := checkRestoreDest(dest); err != nil {
		return nil, err
	}
	if _, err := config.Load(); err != nil {
		return nil, i18n.Errorf("%w; restore records the library there, so fix or remove it first (nothing was restored)", err)
	}
	z, err := zip.OpenReader(p)
	if err != nil {
		return nil, i18n.Errorf("%s is %w: %v", p, ErrBadBackup, err)
	}
	defer z.Close()
	info, byPath, err := checkBackup(&z.Reader, p)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dest)+".restore-*")
	if err != nil {
		return nil, err
	}
	done := false
	defer func() {
		if !done {
			os.RemoveAll(tmp)
		}
	}()
	res := &RestoreResult{Library: dest, Info: info}
	var dirs []BackupFile
	for _, f := range info.Files {
		target := filepath.Join(tmp, filepath.FromSlash(f.Path))
		switch f.Type {
		case "dir":
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, err
			}
			dirs = append(dirs, f)
		case "file":
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, err
			}
			if err := restoreFile(byPath[f.Path], f, target); err != nil {
				return nil, err
			}
		}
	}
	// Links last, so that no file is written through one.
	for _, f := range info.Files {
		if f.Type != "symlink" {
			continue
		}
		t := filepath.FromSlash(f.Target)
		target := filepath.Join(tmp, filepath.FromSlash(f.Path))
		// The target is checked from where the link sits, which a link
		// already on its way would change.
		if filepath.IsAbs(t) || !fsutil.Within(tmp, filepath.Join(filepath.Dir(target), t)) || fsutil.LinkOnPath(tmp, filepath.Dir(target)) != "" {
			res.Skipped = append(res.Skipped, f.Path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if err := os.Symlink(t, target); err != nil {
			return nil, err
		}
	}
	// Directory modes last: a read-only directory is filled first.
	for i := len(dirs) - 1; i >= 0; i-- {
		if m := fs.FileMode(dirs[i].Mode).Perm(); m&0o700 == 0o700 {
			os.Chmod(filepath.Join(tmp, filepath.FromSlash(dirs[i].Path)), m)
		}
	}
	if _, err := library.Open(tmp); err != nil {
		return nil, err
	}
	if err := checkRestoreDest(dest); err != nil {
		return nil, err
	}
	// An empty directory gives way; anything in it would have stopped us.
	os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return nil, err
	}
	done = true
	if err := library.EnsurePrivateGit(dest); err != nil && !errors.Is(err, library.ErrPrivateLink) {
		return res, err
	}
	return res, UseLibrary(dest)
}

// checkRestoreDest refuses a destination that has anything in it.
func checkRestoreDest(dest string) error {
	fi, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.IsDir() {
		list, err := os.ReadDir(dest)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return nil
		}
	}
	return i18n.Errorf("%s already exists and is not empty; nothing was restored. Restore into another directory, or move it to the Trash first", dest)
}

func restoreFile(zf *zip.File, f BackupFile, target string) error {
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := verifyEntry(zf, f, out); err != nil {
		out.Close()
		return i18n.Errorf("%s %v", f.Path, err)
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(target, fs.FileMode(f.Mode).Perm()|0o400)
}
