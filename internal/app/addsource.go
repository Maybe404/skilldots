package app

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/Maybe404/skilldots/internal/manifest"
	"github.com/Maybe404/skilldots/internal/pack"
)

// AddHint is what a pasted `add` source says: the repository, and which
// of its skills the person meant, when the text names some.
//
// Accepted besides owner/repo and git URLs (pack.ParseSource):
//   - an install command copied from a README or a skill directory:
//     `npx skills add <src> [--skill a,b]`, also with bunx, pnpx,
//     `pnpm dlx`, `yarn dlx`, `npm exec`, `bun x`, `npx add-skill <src>` and
//     `skd add <src>`; -s/--skill (repeated, comma separated or several
//     names after one flag) preselects skills, other flags are ignored;
//   - a skills.sh page: https://skills.sh/<owner>/<repo>[/<skill>];
//   - a GitHub link into a repository: /tree/<ref>/<dir> (a skill, or a
//     directory of skills) or /blob/<ref>/<…>/SKILL.md (or any file in
//     a skill).
//
// The text is only parsed, never run. The <ref> in a GitHub link is where
// the page was browsed, not a request to pin: the pack follows the
// repository's default branch as with owner/repo, and `@ref` after the
// repository pins one explicitly.
type AddHint struct {
	// Source is the repository argument, as pack.ParseSource takes it.
	Source string
	// Skills are skill names the text selects.
	Skills []string
	// Paths are directories (or files) inside the repository the text
	// points at; the skills there are selected.
	Paths []string
	// From says what the text was: "" (a plain source), "command",
	// "skills.sh" or "github link".
	From string
}

// runners are the package runners an install command may start with,
// each followed by the words that precede the package.
var runners = [][]string{{"npx"}, {"bunx"}, {"pnpx"}, {"pnpm", "dlx"}, {"pnpm", "exec"}, {"yarn", "dlx"}, {"npm", "exec"}, {"bun", "x"}}

// ParseAddText reads what was typed or pasted as an `add` source.
func ParseAddText(text string) (AddHint, error) {
	text = strings.TrimSpace(firstLine(text))
	for _, p := range []string{"$ ", "% ", "> ", "# "} {
		text = strings.TrimSpace(strings.TrimPrefix(text, p))
	}
	if text == "" {
		return AddHint{}, fmt.Errorf("no source given")
	}
	first := strings.Fields(text)[0]
	if !isCommandWord(first) {
		// One source as typed (a path may contain spaces).
		return parseLink(text)
	}
	words, err := shellWords(text, goos == "windows")
	if err != nil {
		return AddHint{}, err
	}
	h, ok, err := parseCommand(words)
	if err != nil {
		return AddHint{}, err
	}
	if !ok {
		return AddHint{}, fmt.Errorf("%q is not an install command skd understands (try owner/repo, a GitHub or skills.sh link, or `npx skills add …`)", truncateText(text, 60))
	}
	return h, nil
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			return l
		}
	}
	return ""
}

func truncateText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// AddArgs joins the arguments of `skd add` into the text ParseAddText
// reads. An install command pasted without quotes arrives as several
// arguments and is joined; anything else must be one source, so a stray
// extra argument is an error before anything is fetched.
func AddArgs(args []string) (string, error) {
	if len(args) > 1 && !isCommandWord(args[0]) {
		return "", fmt.Errorf("add takes one source, got %d arguments (%s); a path or URL with spaces needs quotes (skd add \"./my skills\"), and so does a pasted install command", len(args), truncateText(strings.Join(args, " "), 60))
	}
	return strings.Join(args, " "), nil
}

func isCommandWord(w string) bool {
	switch strings.ToLower(filepath.Base(w)) {
	case "npx", "bunx", "pnpx", "pnpm", "yarn", "npm", "bun", "skd":
		return true
	}
	return false
}

// parseCommand reads an install command. ok is false when words are not
// one.
func parseCommand(words []string) (h AddHint, ok bool, err error) {
	// Stop at the first shell operator: what follows is another command.
	for i, w := range words {
		if w == "&&" || w == "||" || w == ";" || w == "|" {
			words = words[:i]
			break
		}
	}
	lower := func(i int) string {
		if i < len(words) {
			return strings.ToLower(filepath.Base(words[i]))
		}
		return ""
	}
	i := 0
	switch {
	case lower(0) == "skd":
		if lower(1) != "add" {
			return h, false, nil
		}
		i = 2
	default:
		matched := false
		for _, r := range runners {
			n := 0
			for n < len(r) && lower(n) == r[n] {
				n++
			}
			if n == len(r) {
				i, matched = n, true
				break
			}
		}
		if !matched {
			return h, false, nil
		}
		// The runner's own flags (-y, --yes, --package=x).
		for i < len(words) && strings.HasPrefix(words[i], "-") {
			i++
		}
		pkg := lower(i)
		if at := strings.LastIndex(pkg, "@"); at > 0 {
			pkg = pkg[:at]
		}
		switch pkg {
		case "skills":
			i++
			switch lower(i) {
			case "add", "a", "install", "i":
				i++
			default:
				return h, false, fmt.Errorf("only `skills add` commands can be pasted here (got %q)", strings.Join(words, " "))
			}
		case "add-skill":
			i++
		default:
			return h, false, nil
		}
	}
	h.From = "command"
	var positional []string
	var skills []string
	takeSkills := func(v string) {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				skills = append(skills, s)
			}
		}
	}
	for ; i < len(words); i++ {
		w := words[i]
		name, val, hasVal := strings.Cut(w, "=")
		switch {
		case name == "-s" || name == "--skill" || name == "--skills":
			if hasVal {
				takeSkills(val)
				continue
			}
			// One flag may be followed by several names; a source always
			// has a "/" or ":" and a skill name never does.
			n := 0
			for i+1 < len(words) && !strings.HasPrefix(words[i+1], "-") && !strings.ContainsAny(words[i+1], "/:") {
				i++
				n++
				takeSkills(words[i])
			}
			if n == 0 {
				next := ""
				if i+1 < len(words) {
					next = words[i+1]
				}
				return h, false, fmt.Errorf("%s needs a skill name (got %q)", name, next)
			}
		case name == "-a" || name == "--agent" || name == "--agents":
			if !hasVal {
				for i+1 < len(words) && !strings.HasPrefix(words[i+1], "-") && !strings.ContainsAny(words[i+1], "/:") {
					i++
				}
			}
		case strings.HasPrefix(w, "-"):
			// Other flags (--global, --yes, --all, --list) say nothing
			// about what to install here.
		default:
			positional = append(positional, w)
		}
	}
	if len(positional) == 0 {
		return h, false, fmt.Errorf("the command names no repository")
	}
	src := positional[0]
	for _, p := range positional {
		if strings.ContainsAny(p, "/:") {
			src = p
			break
		}
	}
	link, err := parseLink(src)
	if err != nil {
		return h, false, err
	}
	h.Source, h.Paths = link.Source, link.Paths
	h.Skills = append(link.Skills, skills...)
	for _, s := range h.Skills {
		if err := manifest.CheckName(s); err != nil {
			return h, false, fmt.Errorf("--skill %q: %w", s, err)
		}
	}
	h.Skills = dedupe(h.Skills)
	return h, true, nil
}

// parseLink reads one source: a skills.sh page, a GitHub link into a
// repository, or anything pack.ParseSource takes (returned as is).
func parseLink(s string) (AddHint, error) {
	h := AddHint{Source: s}
	raw := s
	if !strings.Contains(raw, "://") && (strings.HasPrefix(raw, "skills.sh/") || strings.HasPrefix(raw, "www.skills.sh/") ||
		strings.HasPrefix(raw, "github.com/") || strings.HasPrefix(raw, "www.github.com/")) {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return h, nil
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	segs := []string{}
	for _, p := range strings.Split(u.Path, "/") {
		if p != "" {
			segs = append(segs, p)
		}
	}
	switch host {
	case "skills.sh":
		if len(segs) < 2 {
			return h, fmt.Errorf("%s is not a skill page on skills.sh (expected skills.sh/<owner>/<repo>/<skill>)", s)
		}
		h.Source, h.From = segs[0]+"/"+strings.TrimSuffix(segs[1], ".git"), "skills.sh"
		if len(segs) >= 3 {
			name := segs[len(segs)-1]
			if err := manifest.CheckName(name); err != nil {
				return h, fmt.Errorf("%s: %w", s, err)
			}
			h.Skills = []string{name}
		}
		return h, nil
	case "github.com":
		if len(segs) < 2 {
			return h, nil
		}
		repo := segs[0] + "/" + strings.TrimSuffix(segs[1], ".git")
		if len(segs) >= 4 && (segs[2] == "tree" || segs[2] == "blob") {
			// segs[3] is the ref the page was browsed at (ignored, see
			// AddHint); what follows is the path.
			p := strings.Join(segs[4:], "/")
			if segs[2] == "blob" && path.Base(p) == "SKILL.md" {
				p = path.Dir(p)
			}
			h.Source, h.From = "https://github.com/"+repo, "github link"
			if p != "" && p != "." {
				h.Paths = []string{p}
			}
			return h, nil
		}
		if len(segs) > 2 {
			// /issues, /pulls…: the repository is still clear.
			h.Source = "https://github.com/" + repo
		}
	}
	return h, nil
}

// shellWords splits a command line the way a POSIX shell would split
// words, with single and double quotes and backslash escapes, and nothing
// else: no expansion, no substitution, nothing is run. Inside double
// quotes a backslash escapes only $ ` " \ and a line break, as in POSIX.
// With windows set a backslash is never an escape: it separates path
// components (C:\Program Files\…).
func shellWords(s string, windows bool) ([]string, error) {
	var out []string
	var cur strings.Builder
	in := false
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			escaped, in = false, true
			if r == '\n' {
				continue // a line continuation
			}
			if quote == '"' && !strings.ContainsRune("$`\"\\", r) {
				cur.WriteRune('\\')
			}
			cur.WriteRune(r)
		case quote != 0:
			if r == quote {
				quote = 0
			} else if r == '\\' && quote == '"' && !windows {
				escaped = true
			} else {
				cur.WriteRune(r)
			}
		case r == '\\' && !windows:
			escaped, in = true, true
		case r == '\'' || r == '"':
			quote, in = r, true
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote in %q", quote, s)
	}
	if escaped {
		cur.WriteRune('\\')
	}
	if in {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no source given")
	}
	return out, nil
}

// SplitCommand splits a command line (an editor setting, $VISUAL) into
// words like ParseAddText does, without running anything. On Windows a
// backslash is part of a path, not an escape.
func SplitCommand(s string) ([]string, error) { return shellWords(s, goos == "windows") }

// ParseAddText is ParseAdd for anything ParseAddText accepts: it returns
// the source and what the text preselects.
func (s *Session) ParseAddText(text, alias string) (pack.Source, AddHint, error) {
	h, err := ParseAddText(text)
	if err != nil {
		return pack.Source{}, h, err
	}
	src, err := s.ParseAdd(h.Source, alias)
	return src, h, err
}

// Preselect returns the skills of f that h names, in f's order, and the
// names and paths that matched nothing.
func (f *Fetched) Preselect(h AddHint) (names, missing []string) {
	chosen := map[string]bool{}
	for _, n := range h.Skills {
		if sk := pack.Find(f.Skills, n); sk != nil {
			chosen[sk.Name] = true
			continue
		}
		// A link names the directory, which may differ from the name.
		found := false
		for _, sk := range f.Skills {
			if filepath.Base(sk.Dir) == n {
				chosen[sk.Name], found = true, true
			}
		}
		if !found {
			missing = append(missing, n)
		}
	}
	for _, p := range h.Paths {
		hits := f.skillsAt(p)
		if len(hits) == 0 {
			missing = append(missing, p)
		}
		for _, n := range hits {
			chosen[n] = true
		}
	}
	for _, sk := range f.Skills {
		if chosen[sk.Name] {
			names = append(names, sk.Name)
		}
	}
	return names, missing
}

// skillsAt are the skills at repository path p: the skill whose directory
// p is or is inside, or every skill under p. A ref with "/" in a GitHub
// link leaves its tail at the front of p, so leading segments are dropped
// until something matches.
func (f *Fetched) skillsAt(p string) []string {
	p = strings.Trim(path.Clean("/"+p), "/")
	segs := strings.Split(p, "/")
	for i := range segs {
		cand := strings.Join(segs[i:], "/")
		var hits []string
		for _, sk := range f.Skills {
			dirs := append([]string{sk.Dir}, sk.AltDirs...)
			for _, d := range dirs {
				rel, err := filepath.Rel(f.Clone, d)
				if err != nil {
					continue
				}
				rel = filepath.ToSlash(rel)
				if rel == cand || strings.HasPrefix(cand, rel+"/") || strings.HasPrefix(rel, cand+"/") {
					hits = append(hits, sk.Name)
					break
				}
			}
		}
		if len(hits) > 0 {
			return hits
		}
	}
	return nil
}
