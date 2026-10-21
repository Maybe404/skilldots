package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Maybe404/skilldots/internal/engine"
	"github.com/Maybe404/skilldots/internal/fsutil"
	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/library"
	"github.com/Maybe404/skilldots/internal/lock"
	"github.com/Maybe404/skilldots/internal/pack"
)

// UpdateDiff is what `skd update` would do to one pack's vendored skills,
// file by file, worked out before anything changes: for a skill the user
// has not changed, the library's copy against the upstream copy update
// puts in its place; for one the user changed, what upstream changed
// since the version those changes were made on (update merges that into
// them), with the files expected to conflict marked. Nothing in the
// library is written.
type UpdateDiff struct {
	Repo   string      `json:"repo"`
	Commit string      `json:"commit"`
	Latest string      `json:"latest"`
	Skills []SkillDiff `json:"skills"`
}

// SkillDiff is one skill update would replace or vendor.
type SkillDiff struct {
	Name string `json:"name"`
	// Status is "changed" (upstream changed it), "added" (selected but not
	// vendored yet) or "moved" (only its category changed upstream).
	Status string `json:"status"`
	// Local is set when the library's copy is not what skd vendored (the
	// user edited it): update merges upstream's changes into it (the copy
	// as it was goes to the trash first), or shelves the update when they
	// conflict.
	Local bool `json:"local"`
	// Base is set when Local is and the version the user's changes were
	// made on was found: Files then compare it with the new upstream
	// version (what the merge brings in), not the user's copy with it.
	// Without it Files compare the user's copy with upstream, as before.
	Base bool `json:"base,omitempty"`
	// Conflicts are the files the merge is expected to conflict on (the
	// update of the skill is then shelved until skd resolve); worked out
	// only when Base is set.
	Conflicts []string `json:"conflicts,omitempty"`
	// LocalFiles are the files the user changed, added or removed, when
	// the vendored version could be found in the library's git history.
	LocalFiles []string `json:"localFiles"`
	// LocalUnknown is set when Local is but which files changed is not
	// known (the vendored version was never committed).
	LocalUnknown bool       `json:"localUnknown,omitempty"`
	Files        []FileDiff `json:"files"`
	// MoreFiles counts changed files left out (MaxDiffFiles).
	MoreFiles int `json:"moreFiles,omitempty"`
}

// FileDiff is one file of a skill that differs.
type FileDiff struct {
	// Path is relative to the skill's directory, with forward slashes.
	Path string `json:"path"`
	// Status is "added", "removed" or "modified".
	Status string `json:"status"`
	// Local marks a file the user changed in the library's copy: with the
	// skill's Base set, a file whose upstream change is merged into the
	// user's (or conflicts with it, Conflict).
	Local    bool `json:"local,omitempty"`
	Conflict bool `json:"conflict,omitempty"`
	// Binary or TooLarge files carry no hunks.
	Binary   bool       `json:"binary,omitempty"`
	TooLarge bool       `json:"tooLarge,omitempty"`
	Added    int        `json:"added"`
	Removed  int        `json:"removed"`
	Hunks    []DiffHunk `json:"hunks"`
}

// DiffHunk is a run of changed lines with up to diffContext lines of
// context around it, numbered from 1 as in a unified diff.
type DiffHunk struct {
	OldStart int        `json:"oldStart"`
	OldLines int        `json:"oldLines"`
	NewStart int        `json:"newStart"`
	NewLines int        `json:"newLines"`
	Lines    []DiffLine `json:"lines"`
}

// DiffLine is one line of a hunk: Op is " " (context), "-" or "+".
type DiffLine struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

const (
	diffContext = 3
	// MaxDiffFiles bounds the files listed per skill.
	MaxDiffFiles = 200
	// maxDiffBytes bounds a file compared line by line.
	maxDiffBytes = 512 << 10
	// maxDiffCells bounds the line-by-line comparison (lines × lines).
	maxDiffCells = 4_000_000
	// baselineCommits bounds the library commits searched for the
	// vendored version of an edited skill.
	baselineCommits = 30
)

// UpdateDiff compares the vendored copies of repo's selected skills with
// upstream. latest is the upstream commit the caller last saw (from a
// check): when the cache clone is at it nothing is fetched; otherwise (or
// when latest is "") upstream is fetched first, as a check does.
func (s *Session) UpdateDiff(ctx context.Context, repo, latest string) (*UpdateDiff, error) {
	p := s.Lib.Manifest.FindPack(repo)
	if p == nil {
		return nil, i18n.Errorf("no pack %q", repo)
	}
	packDir, err := engine.PackDir(s.Lib, p.Repo)
	if err != nil {
		return nil, err
	}
	src, err := pack.ParseSource(p.Repo)
	if err != nil {
		return nil, err
	}
	src.Ref = p.Ref
	unlock, err := pack.HoldContext(ctx, p.Repo)
	defer unlock()
	if err != nil {
		return nil, err
	}
	clone, commit := pack.CachePath(p.Repo), ""
	if latest != "" && fsutil.Exists(filepath.Join(clone, ".git")) {
		if head, err := library.GitContext(ctx, clone, "rev-parse", "HEAD"); err == nil && head == latest {
			commit = head
		}
	}
	if commit == "" {
		if clone, commit, err = pack.FetchContext(ctx, src); err != nil {
			return nil, err
		}
	}
	upstream, _, err := pack.Scan(clone)
	if err != nil {
		return nil, err
	}
	out := &UpdateDiff{Repo: p.Repo, Latest: commit, Skills: []SkillDiff{}}
	lp := s.Lib.Lock.Packs[p.Repo]
	vendored := map[string]*lock.Skill{}
	if lp != nil {
		out.Commit = lp.Commit
		if lp.Skills != nil {
			vendored = lp.Skills
		}
	}
	for _, u := range upstream {
		if !p.Selected(u.Name, u.Category) {
			continue
		}
		ls := vendored[u.Name]
		if ls != nil {
			// Hashed as it streams: an unchanged skill is never read.
			if h, err := fsutil.HashTree(u.Dir); err == nil && h == ls.Hash {
				if ls.Category != u.Category {
					out.Skills = append(out.Skills, SkillDiff{Name: u.Name, Status: "moved", LocalFiles: []string{}, Files: []FileDiff{}})
				}
				continue
			}
		}
		newFiles, err := readFiles(u.Dir)
		if err != nil {
			return nil, err
		}
		if ls == nil {
			sd := SkillDiff{Name: u.Name, Status: "added", LocalFiles: []string{}}
			sd.Files, sd.MoreFiles = diffFiles(nil, newFiles, nil)
			out.Skills = append(out.Skills, sd)
			continue
		}
		dir := filepath.Join(packDir, filepath.FromSlash(ls.Category), u.Name)
		if !fsutil.Within(packDir, dir) {
			return nil, i18n.Errorf("%s is outside %s", dir, packDir)
		}
		oldFiles := map[string][]byte{}
		if fsutil.Exists(dir) {
			if oldFiles, err = readFiles(dir); err != nil {
				return nil, err
			}
		}
		sd := SkillDiff{Name: u.Name, Status: "changed", LocalFiles: []string{}}
		local := map[string]bool{}
		if fsutil.Exists(dir) && !fsutil.Pristine(dir, ls.Hash) {
			sd.Local = true
			if m := s.mergePreview(p.Repo, u, dir, ls, commit); m != nil {
				out.Skills = append(out.Skills, *m)
				continue
			}
			if base := s.vendoredBaseline(ctx, dir, ls.Hash); base != nil {
				for _, f := range changedPaths(base, oldFiles) {
					local[f] = true
					sd.LocalFiles = append(sd.LocalFiles, f)
				}
			} else {
				sd.LocalUnknown = true
			}
		}
		sd.Files, sd.MoreFiles = diffFiles(oldFiles, newFiles, local)
		out.Skills = append(out.Skills, sd)
	}
	return out, nil
}

// mergePreview is the diff of a skill the user changed as update will
// merge it: upstream's version the changes were made on (the lock's
// hash, found in the snapshots or the library's history) against the
// new one, the files the user changed marked, and those the merge would
// conflict on. nil when that version is not found.
func (s *Session) mergePreview(repo string, u pack.Skill, dir string, ls *lock.Skill, commit string) *SkillDiff {
	base := engine.FindBase(s.Lib, repo, u.Name, []string{ls.Category, u.Category}, ls.Hash)
	if base == nil {
		return nil
	}
	ours, err := engine.ReadFileSet(dir)
	if err != nil {
		return nil
	}
	theirs, err := engine.ReadFileSet(u.Dir)
	if err != nil {
		return nil
	}
	m, err := engine.Merge(base, ours, theirs, commit)
	if err != nil {
		return nil
	}
	sd := &SkillDiff{Name: u.Name, Status: "changed", Local: true, Base: true, LocalFiles: []string{}, Conflicts: m.ConflictPaths()}
	baseFiles, ourFiles, newFiles := diffable(base), diffable(ours), diffable(theirs)
	local := map[string]bool{}
	for _, f := range changedPaths(baseFiles, ourFiles) {
		local[f] = true
		sd.LocalFiles = append(sd.LocalFiles, f)
	}
	sd.Files, sd.MoreFiles = diffFiles(baseFiles, newFiles, local)
	for i := range sd.Files {
		sd.Files[i].Conflict = slices.Contains(sd.Conflicts, sd.Files[i].Path)
	}
	return sd
}

// diffable is a file set as readFiles reads one: a file larger than
// maxDiffBytes is its tooLargeMark.
func diffable(fs engine.FileSet) map[string][]byte {
	out := map[string][]byte{}
	for p, f := range fs {
		if len(f.Data) > maxDiffBytes {
			sum := sha256.Sum256(f.Data)
			out[p] = tooLargeMark(sum[:])
		} else {
			out[p] = f.Data
		}
	}
	return out
}

// tooLargePrefix starts what a set of files read for a diff holds for a
// file larger than maxDiffBytes: the prefix and the file's SHA-256, so
// two such files still compare equal or not, without their content
// being held. diffFiles shows it as too large; a set with one does not
// hash like the files themselves (hashFiles), so which files the user
// changed in such a skill is not known.
var tooLargePrefix = []byte("\x00skd:too-large:")

func tooLargeMark(sum []byte) []byte {
	return append(append([]byte{}, tooLargePrefix...), hex.EncodeToString(sum)...)
}

func isTooLarge(b []byte) bool { return bytes.HasPrefix(b, tooLargePrefix) }

// readFiles reads every regular file under dir as fsutil.HashTree sees
// them (Skip entries left out), keyed by slash-separated relative path.
// A file larger than maxDiffBytes is not held: its tooLargeMark is.
func readFiles(dir string) (map[string][]byte, error) {
	if filesRead != nil {
		filesRead(dir)
	}
	files := map[string][]byte{}
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
		if info.Size() > maxDiffBytes {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			h := sha256.New()
			_, err = io.Copy(h, f)
			f.Close()
			if err != nil {
				return err
			}
			files[filepath.ToSlash(rel)] = tooLargeMark(h.Sum(nil))
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = b
		return nil
	})
	return files, err
}

// filesRead, when set (tests), is told each directory readFiles reads.
var filesRead func(dir string)

// hashFiles is fsutil.HashTree over files already read.
func hashFiles(files map[string][]byte) string {
	names := sortedNames(files)
	h := sha256.New()
	for _, n := range names {
		io.WriteString(h, n)
		h.Write([]byte{0})
		h.Write(files[n])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func sortedNames(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// changedPaths lists the paths whose content differs between a and b,
// including those only one of them has.
func changedPaths(a, b map[string][]byte) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string][]byte{a, b} {
		for n := range m {
			if seen[n] {
				continue
			}
			seen[n] = true
			x, inA := a[n]
			y, inB := b[n]
			if inA != inB || !bytes.Equal(x, y) {
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// vendoredBaseline finds the files skd vendored into dir (whose hash the
// lock records) in the library's git history: the newest of the last few
// commits touching dir whose copy hashes to hash. nil when none does.
func (s *Session) vendoredBaseline(ctx context.Context, dir, hash string) map[string][]byte {
	rel := s.Lib.Rel(dir)
	if rel == dir {
		return nil
	}
	log, err := library.GitContext(ctx, s.Lib.Root, "log", "-n", strconv.Itoa(baselineCommits), "--format=%H", "--", rel)
	if err != nil || log == "" {
		return nil
	}
	for _, c := range strings.Fields(log) {
		files, err := gitTreeFiles(ctx, s.Lib.Root, c, rel)
		if err != nil || len(files) == 0 {
			continue
		}
		if hashFiles(files) == hash {
			return files
		}
	}
	return nil
}

// gitTreeFiles reads the regular files under rel at commit c, keyed by
// their path relative to rel, leaving out fsutil.Skip names as HashTree
// does. The answer is read as it comes, never held whole: a blob larger
// than maxDiffBytes is only hashed (tooLargeMark).
func gitTreeFiles(ctx context.Context, root, c, rel string) (map[string][]byte, error) {
	blobs, err := engine.GitTreeBlobs(ctx, root, c, rel)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	err = engine.ReadGitBlobs(ctx, root, blobs, maxDiffBytes, func(b engine.GitBlob, data, sum []byte) error {
		if sum != nil {
			files[b.Path] = tooLargeMark(sum)
		} else {
			files[b.Path] = data
		}
		return nil
	})
	return files, err
}

// diffFiles compares two copies of a skill, file by file. local marks
// the files the user changed. At most MaxDiffFiles are returned; the
// rest are counted.
func diffFiles(old, new map[string][]byte, local map[string]bool) ([]FileDiff, int) {
	out := []FileDiff{}
	more := 0
	for _, n := range changedPaths(old, new) {
		if len(out) == MaxDiffFiles {
			more++
			continue
		}
		a, inOld := old[n]
		b, inNew := new[n]
		fd := FileDiff{Path: n, Status: "modified", Local: local[n], Hunks: []DiffHunk{}}
		switch {
		case !inOld:
			fd.Status = "added"
		case !inNew:
			fd.Status = "removed"
		}
		switch {
		case isTooLarge(a) || isTooLarge(b):
			fd.TooLarge = true
		case isBinary(a) || isBinary(b):
			fd.Binary = true
		case len(a) > maxDiffBytes || len(b) > maxDiffBytes:
			fd.TooLarge = true
		default:
			fd.Hunks, fd.Added, fd.Removed = lineDiff(splitLines(a), splitLines(b))
			if fd.Hunks == nil {
				fd.TooLarge = true
				fd.Hunks = []DiffHunk{}
			}
		}
		out = append(out, fd)
	}
	return out, more
}

func isBinary(b []byte) bool {
	return bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b)
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	s := strings.TrimSuffix(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	return strings.Split(s, "\n")
}

// lineDiff is a unified diff of a and b by longest common subsequence,
// as hunks with diffContext lines of context, and the number of lines
// added and removed. It returns nil hunks when the files are too long to
// compare (maxDiffCells).
func lineDiff(a, b []string) ([]DiffHunk, int, int) {
	// Common head and tail need no table.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]
	if len(ma)*len(mb) > maxDiffCells {
		return nil, 0, 0
	}
	// lcs[i][j] is the common subsequence length of ma[i:] and mb[j:].
	w := len(mb) + 1
	lcs := make([]int32, (len(ma)+1)*w)
	for i := len(ma) - 1; i >= 0; i-- {
		for j := len(mb) - 1; j >= 0; j-- {
			if ma[i] == mb[j] {
				lcs[i*w+j] = lcs[(i+1)*w+j+1] + 1
			} else {
				lcs[i*w+j] = max(lcs[(i+1)*w+j], lcs[i*w+j+1])
			}
		}
	}
	type op struct {
		op         string
		text       string
		oldN, newN int // line numbers (from 1) before this line in each file
	}
	var ops []op
	oi, ni := 0, 0
	push := func(o, text string) {
		ops = append(ops, op{o, text, oi, ni})
		switch o {
		case " ":
			oi++
			ni++
		case "-":
			oi++
		case "+":
			ni++
		}
	}
	for _, l := range a[:pre] {
		push(" ", l)
	}
	i, j := 0, 0
	for i < len(ma) || j < len(mb) {
		switch {
		case i < len(ma) && j < len(mb) && ma[i] == mb[j]:
			push(" ", ma[i])
			i++
			j++
		// On a tie the removed line comes first, as in git's output.
		case i < len(ma) && (j == len(mb) || lcs[(i+1)*w+j] >= lcs[i*w+j+1]):
			push("-", ma[i])
			i++
		default:
			push("+", mb[j])
			j++
		}
	}
	for _, l := range a[len(a)-suf:] {
		push(" ", l)
	}
	added, removed := 0, 0
	for _, o := range ops {
		switch o.op {
		case "+":
			added++
		case "-":
			removed++
		}
	}
	// Group the changes into hunks, merging those diffContext lines apart.
	hunks := []DiffHunk{}
	for k := 0; k < len(ops); {
		if ops[k].op == " " {
			k++
			continue
		}
		start := max(0, k-diffContext)
		end := k
		for end < len(ops) {
			if ops[end].op != " " {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].op == " " {
				run++
			}
			if run == len(ops) || run-end > 2*diffContext {
				end = min(len(ops), end+diffContext)
				break
			}
			end = run
		}
		h := DiffHunk{OldStart: ops[start].oldN + 1, NewStart: ops[start].newN + 1}
		for _, o := range ops[start:end] {
			h.Lines = append(h.Lines, DiffLine{o.op, o.text})
			if o.op != "+" {
				h.OldLines++
			}
			if o.op != "-" {
				h.NewLines++
			}
		}
		// A side with no lines starts, as in a unified diff, at the line before.
		if h.OldLines == 0 {
			h.OldStart--
		}
		if h.NewLines == 0 {
			h.NewStart--
		}
		hunks = append(hunks, h)
		k = end
	}
	return hunks, added, removed
}
