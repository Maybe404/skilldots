package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/pack"
)

// Upstream snapshots: after every update of a pack, the library's git
// repository gets a commit holding what a pristine vendor/<pack> would
// hold (the selected skills as category/name/…, and the repository's
// LICENSE), on a ref of its own, refs/skilldots/upstream/<pack>. Each
// snapshot's parent is the one before, so the ref's history is every
// upstream version the library has vendored. It is the merge base of
// the next update of a skill the user changed, and what `skd history
// --diff` compares. The library's index, work tree and branches are not
// touched.

// SnapshotRefs is the namespace of the snapshot refs; `skd sync` pushes it.
const SnapshotRefs = "refs/skilldots/"

const snapshotPrefix = SnapshotRefs + "upstream/"

// snapshotIdentity is the author and committer of every snapshot: they
// are made by skd, not by whoever runs it, and need no git identity.
var snapshotIdentity = []string{"GIT_AUTHOR_NAME=skilldots", "GIT_AUTHOR_EMAIL=skilldots@localhost",
	"GIT_COMMITTER_NAME=skilldots", "GIT_COMMITTER_EMAIL=skilldots@localhost"}

// maxSnapshots bounds the snapshots searched for a merge base, and
// baseCommits the library commits searched after them.
const (
	maxSnapshots = 200
	baseCommits  = 30
)

// KeepsSnapshots reports whether the library can keep upstream snapshots:
// it is the top of a git work tree of its own.
func KeepsSnapshots(lib *library.Library) bool { return library.IsGitRepo(lib.Root) }

// SnapshotRef is the ref holding repo's snapshots in lib: the pack's key
// as ref path segments when git accepts that, else a name made from its
// hash.
func SnapshotRef(lib *library.Library, repo string) string {
	ref := snapshotPrefix + repo
	if _, err := library.Git(lib.Root, "check-ref-format", ref); err == nil && !strings.HasPrefix(repo, "-") {
		return ref
	}
	sum := sha256.Sum256([]byte(repo))
	return snapshotPrefix + "x-" + hex.EncodeToString(sum[:8])
}

// SnapshotTip is the newest snapshot of repo, "" when there is none.
func SnapshotTip(lib *library.Library, repo string) string {
	out, err := library.Git(lib.Root, "rev-parse", "--verify", "-q", SnapshotRef(lib, repo)+"^{commit}")
	if err != nil {
		return ""
	}
	return out
}

// gitInput runs git in dir with env added and stdin fed, returning its
// trimmed output.
func gitInput(dir string, env []string, stdin []byte, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = bytes.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", i18n.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// snapshotFile is one file of a snapshot: where it is read from and its
// path in the snapshot's tree.
type snapshotFile struct {
	src, path string
	exec      bool
}

// filesOf lists the files of a skill's upstream directory as CopyTree
// copies them, placed under prefix.
func filesOf(dir, prefix string) ([]snapshotFile, error) {
	var out []snapshotFile
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if fsutil.Skip[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, snapshotFile{src: p, path: prefix + filepath.ToSlash(rel), exec: info.Mode().Perm()&0o111 != 0})
		return nil
	})
	return out, err
}

// writeSnapshot records the upstream skills a pristine vendor/<repo>
// holds (skills, and license, the repository's LICENSE file, when set)
// at upstream commit as the next snapshot of repo. It returns the
// snapshot before and the one after ("" when there was none); when
// nothing changed since the last one, both are that one. Its date is
// date (upstream's commit's), so the same snapshot made on two machines
// is the same commit.
func writeSnapshot(lib *library.Library, repo, commit, date string, skills []pack.Skill, license string) (from, to string, err error) {
	from = SnapshotTip(lib, repo)
	var files []snapshotFile
	for _, s := range skills {
		prefix := s.Name + "/"
		if s.Category != "" {
			prefix = s.Category + "/" + prefix
		}
		fs, err := filesOf(s.Dir, prefix)
		if err != nil {
			return from, "", err
		}
		files = append(files, fs...)
	}
	if license != "" {
		files = append(files, snapshotFile{src: license, path: filepath.Base(license)})
	}
	var paths bytes.Buffer
	for _, f := range files {
		if strings.ContainsAny(f.src, "\n\r") || strings.ContainsAny(f.path, "\x00") {
			return from, "", i18n.Errorf("%s: a file name with a line break cannot be recorded", f.src)
		}
		paths.WriteString(f.src + "\n")
	}
	tmp, err := os.MkdirTemp("", "skd-snapshot-")
	if err != nil {
		return from, "", err
	}
	defer os.RemoveAll(tmp)
	// A temporary index: the library's own is never touched.
	env := append([]string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}, snapshotIdentity...)
	if date != "" {
		env = append(env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	}
	var index bytes.Buffer
	if len(files) > 0 {
		// --no-filters: stored as the files are, whatever .gitattributes says.
		out, err := gitInput(lib.Root, env, paths.Bytes(), "hash-object", "-w", "--no-filters", "--stdin-paths")
		if err != nil {
			return from, "", err
		}
		shas := strings.Fields(out)
		if len(shas) != len(files) {
			return from, "", i18n.Errorf("git hash-object answered %d objects for %d files", len(shas), len(files))
		}
		for i, f := range files {
			mode := "100644"
			if f.exec {
				mode = "100755"
			}
			index.WriteString(mode + " " + shas[i] + "\t" + f.path + "\x00")
		}
	}
	if _, err := gitInput(lib.Root, env, index.Bytes(), "update-index", "-z", "--add", "--index-info"); err != nil {
		return from, "", err
	}
	tree, err := gitInput(lib.Root, env, nil, "write-tree")
	if err != nil {
		return from, "", err
	}
	if from != "" {
		if old, err := library.Git(lib.Root, "rev-parse", from+"^{tree}"); err == nil && old == tree {
			return from, from, nil
		}
	}
	args := []string{"commit-tree", "--no-gpg-sign", tree, "-m", "upstream " + repo + " " + commit}
	if from != "" {
		args = append(args, "-p", from)
	}
	to, err = gitInput(lib.Root, env, nil, args...)
	if err != nil {
		return from, "", err
	}
	upd := []string{"update-ref", "-m", "skd: upstream snapshot", SnapshotRef(lib, repo), to}
	if from != "" {
		upd = append(upd, from)
	}
	if _, err := gitInput(lib.Root, nil, nil, upd...); err != nil {
		return from, "", err
	}
	return from, to, nil
}

// commitDate is the committer date of commit in the clone at dir, ""
// when it cannot be read.
func commitDate(dir, commit string) string {
	out, err := library.Git(dir, "log", "-1", "--format=%cI", commit)
	if err != nil {
		return ""
	}
	return out
}

// skillTrees is the tree of path in each of commits (missing ones left
// out), in their order, without repeating a tree.
func skillTrees(root string, commits []string, paths []string) []string {
	var in bytes.Buffer
	for _, c := range commits {
		for _, p := range paths {
			in.WriteString(c + ":" + p + "\n")
		}
	}
	out, err := gitInput(root, nil, in.Bytes(), "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return nil
	}
	var trees []string
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && f[1] == "tree" && !seen[f[0]] {
			seen[f[0]] = true
			trees = append(trees, f[0])
		}
	}
	return trees
}

// findTree is the first of trees whose files hash to hash, read whole.
func findTree(root string, trees []string, hash string) FileSet {
	for _, t := range trees {
		files, err := ReadGitFileSet(context.Background(), root, t, "")
		if err == nil && len(files) > 0 && files.Hash() == hash {
			return files
		}
	}
	return nil
}

// FindBase is the version of a vendored skill whose content hash is
// hash (the lock's: what was vendored before the local changes): from
// repo's snapshots, else from the library's own history of the skill's
// directory (vendor/<repo>/<category>/<name>, for each category the skill
// may have had). nil when neither has it.
func FindBase(lib *library.Library, repo, name string, categories []string, hash string) FileSet {
	if hash == "" || !KeepsSnapshots(lib) {
		return nil
	}
	var inPack, inLib []string
	packDir, err := PackDir(lib, repo)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, c := range categories {
		if seen[c] {
			continue
		}
		seen[c] = true
		p := name
		if c != "" {
			p = c + "/" + name
		}
		inPack = append(inPack, p)
		inLib = append(inLib, lib.Rel(filepath.Join(packDir, filepath.FromSlash(p))))
	}
	if tip := SnapshotTip(lib, repo); tip != "" {
		if out, err := library.Git(lib.Root, "rev-list", "--max-count="+strconv.Itoa(maxSnapshots), tip); err == nil {
			if f := findTree(lib.Root, skillTrees(lib.Root, strings.Fields(out), inPack), hash); f != nil {
				return f
			}
		}
	}
	args := append([]string{"log", "-n", strconv.Itoa(baseCommits), "--format=%H", "--"}, inLib...)
	if out, err := library.Git(lib.Root, args...); err == nil && out != "" {
		return findTree(lib.Root, skillTrees(lib.Root, strings.Fields(out), inLib), hash)
	}
	return nil
}

// FindSkillDir is the directory of the tree-ish rev (of the repository
// at root) that holds the skill name: prefer when it has a SKILL.md, else
// the shortest directory named name that has one; "" when none does.
func FindSkillDir(root, rev, name, prefer string) (string, error) {
	out, err := library.Git(root, "ls-tree", "-r", "--name-only", "--full-tree", rev)
	if err != nil {
		return "", err
	}
	found := ""
	for _, p := range strings.Split(out, "\n") {
		dir, isSkill := strings.CutSuffix(p, "/SKILL.md")
		if !isSkill {
			continue
		}
		if dir == prefer && prefer != "" {
			return dir, nil
		}
		if (dir == name || strings.HasSuffix(dir, "/"+name)) && (found == "" || len(dir) < len(found)) {
			found = dir
		}
	}
	return found, nil
}

// SnapshotSkill reads a skill of repo's snapshot rev (a commit), looked
// for at category/name first, else wherever a directory named name with
// a SKILL.md is. It returns the files and the category they are under;
// nil files when the snapshot does not have the skill.
func SnapshotSkill(lib *library.Library, rev, name, category string) (FileSet, string, error) {
	want := name
	if category != "" {
		want = category + "/" + name
	}
	found, err := FindSkillDir(lib.Root, rev, name, want)
	if err != nil || found == "" {
		return nil, "", err
	}
	files, err := ReadGitFileSet(context.Background(), lib.Root, rev, found)
	if err != nil {
		return nil, "", err
	}
	cat, _ := strings.CutSuffix(found, name)
	return files, strings.TrimSuffix(cat, "/"), nil
}
