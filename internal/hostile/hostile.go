// Package hostile builds the "most hostile" library the regression tests
// run skd against: every piece of text skd reads from outside (an
// upstream repository, the library's files, agent directories, the trash,
// the settings file) carries control characters, escape sequences and
// bidi controls. Nothing of it may reach a terminal. It is only used by
// tests.
package hostile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Evil holds one of everything that must never reach a terminal: an OSC
// that sets the window title, BEL, a CSI that clears the screen, an
// OSC 52 that writes the clipboard, the C1 CSI, DEL and the bidi controls
// that reorder text. It holds no NUL and no "/", so it can be part of a
// file name.
const Evil = "\x1b]0;pwned\x07\x1b[2J\x1b]52;c;cHduZWQ=\x07\u009b31m\u202e\u2066x\u2069\u200f\x7f\x08\r\t"

// Normal is text that is not hostile and must come through intact:
// Chinese, emoji, spaces.
const Normal = "中文 描述 🚀 with spaces"

// TOML is s as a TOML basic string, everything but printable ASCII
// escaped, so the file stays valid and carries s exactly.
func TOML(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		case r > 0xffff:
			fmt.Fprintf(&b, `\U%08X`, r)
		default:
			fmt.Fprintf(&b, `\u%04X`, r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Git runs git in dir; the test supplies it (with its own environment).
type Git func(dir string, args ...string)

func write(path, body string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		panic(err)
	}
}

func skillMD(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\nlicense: " + Evil + "\nmetadata: " + Evil + "\n---\n" + Evil + "\n"
}

// Upstream makes dir a git repository of skills whose descriptions,
// licence, frontmatter, directory names, categories and commit subjects
// carry Evil. Skills: tdd and review (category eng, review with a Normal
// description), weird (in a directory named with Evil, and a second copy
// with other content in another one), and two that skd must reject: one in a category named with Evil, one whose name has it.
func Upstream(dir string, git Git) {
	write(filepath.Join(dir, "LICENSE"), "MIT License "+Evil+"\n\nPermission is hereby granted\n")
	write(filepath.Join(dir, "skills", "eng", "tdd", "SKILL.md"), skillMD("tdd", "tests first "+Evil))
	write(filepath.Join(dir, "skills", "eng", "review", "SKILL.md"), skillMD("review", Normal))
	write(filepath.Join(dir, "skills", "x"+Evil, "SKILL.md"), skillMD("weird", Evil))
	// Another copy of weird, with other content: show names both
	// directories.
	write(filepath.Join(dir, "x", "weird", "SKILL.md"), skillMD("weird", "other "+Evil))
	write(filepath.Join(dir, "skills", "cat"+Evil, "hidden", "SKILL.md"), skillMD("hidden", Evil))
	write(filepath.Join(dir, "skills", "badname", "SKILL.md"), skillMD("bad"+Evil, Evil))
	git(dir, "init", "-q")
	git(dir, "add", "-A")
	git(dir, "commit", "-qm", "init "+Evil)
}

// Change commits upstream changes to Upstream's repository: tdd changes,
// a skill appears (one more with an Evil category), with Evil in the
// subject, so update, outdated and history have something to report.
func Change(dir string, git Git) {
	write(filepath.Join(dir, "skills", "eng", "tdd", "more.md"), Evil)
	write(filepath.Join(dir, "skills", "eng", "fresh", "SKILL.md"), skillMD("fresh", Evil))
	write(filepath.Join(dir, "skills", "new"+Evil, "later", "SKILL.md"), skillMD("later", Evil))
	git(dir, "add", "-A")
	git(dir, "commit", "-qm", "change "+Evil)
}

// Lures are trash entries a hostile library holds so that restoring
// one would move a directory to, or from, where skd never puts one.
type Lures struct {
	// IDs are the `skd trash restore` arguments that must be refused.
	IDs []string
	// Never are paths that must not exist after any of them.
	Never []string
	// Kept are the lured directories that must stay where they are.
	Kept []string
}

// Extras adds what other files of a library and of the machine can
// carry: a history whose every field (and one it does not know) holds
// Evil, trash entries named with it whose recorded origin holds it, and
// in agentDir an unmanaged skill, a copy and a broken link named or
// described with it. home is where the paths in the trash point. It
// also adds the trash entries of Lures and returns them.
func Extras(root, agentDir, home string) Lures {
	rec := map[string]any{
		"time": time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), "repo": "x/y" + Evil, "from": "a" + Evil, "to": "b" + Evil,
		"changed": []string{"c" + Evil}, "added": []string{"a" + Evil}, "moved": []string{"m" + Evil},
		"gone": []string{"g" + Evil}, "dropped": []string{"d" + Evil}, "new": []string{"n" + Evil},
		"commits":    []map[string]string{{"hash": "h" + Evil, "subject": "s" + Evil}},
		"applyError": "link failed " + Evil, "omitted": 3, "unknown" + Evil: Evil,
	}
	line, _ := json.Marshal(rec)
	f, err := os.OpenFile(filepath.Join(root, "skills.history.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		panic(err)
	}
	f.Write(append(line, '\n'))
	f.Close()

	for _, batch := range []string{"20240102-030405", "b" + Evil} {
		d := filepath.Join(root, ".trash", batch, "t"+Evil)
		write(filepath.Join(d, "SKILL.md"), skillMD("t", Evil))
		write(d+".origin", filepath.Join(home, ".agents", "skills", "o"+Evil)+"\n")
	}

	write(filepath.Join(agentDir, "odd"+Evil, "SKILL.md"), skillMD("odd"+Evil, Evil))
	write(filepath.Join(agentDir, "copy", "SKILL.md"), skillMD("copy", Evil))
	os.Symlink(filepath.Join(home, "gone"+Evil), filepath.Join(agentDir, "lnk"+Evil))
	return lures(root, agentDir, home)
}

// lures parks, in batch 20240102-030406, entries recorded as coming from
// outside every place skd moves directories from (under ~/Library, a
// relative path, a path with "..", the library's skills/), and reaches
// directories outside the library: by an id with "..", through a batch
// that is a link, and through an entry that is a link. Those directories
// have records naming a place skd would accept.
func lures(root, agentDir, home string) Lures {
	var l Lures
	park := func(dir, name, origin string) string {
		p := filepath.Join(dir, name)
		write(filepath.Join(p, "SKILL.md"), skillMD(name, "lure"))
		write(p+".origin", origin+"\n")
		return p
	}
	batch := filepath.Join(root, ".trash", "20240102-030406")
	for name, origin := range map[string]string{
		"library": filepath.Join(home, "Library", "LaunchAgents", "lure"),
		"rel":     filepath.Join("rel", "lure"),
		"dotdot":  agentDir + "/../../Library/dotdot", // filepath.Join would clean it
		"mine":    filepath.Join(root, "skills", "lure-mine"),
		"evil":    filepath.Join(home, "Library", "x"+Evil),
	} {
		park(batch, name, origin)
		l.IDs = append(l.IDs, "20240102-030406/"+name)
		l.Kept = append(l.Kept, filepath.Join(batch, name))
		if !filepath.IsAbs(origin) {
			origin = filepath.Join(root, origin) // skd runs in the library
		}
		l.Never = append(l.Never, filepath.Clean(origin))
	}
	// Not ~/Library itself: the settings file is there on macOS.
	l.Never = append(l.Never, filepath.Join(home, "Library", "LaunchAgents"), filepath.Join(home, "Library", "dotdot"))

	// Outside the library, with records skd would accept.
	outside := filepath.Join(home, "outside")
	victim := park(outside, "victim", filepath.Join(agentDir, "victim"))
	l.Kept = append(l.Kept, victim)
	l.Never = append(l.Never, filepath.Join(agentDir, "victim"))
	up, _ := filepath.Rel(filepath.Join(root, ".trash"), victim)
	os.Symlink(outside, filepath.Join(root, ".trash", "20240102-030407"))
	os.MkdirAll(filepath.Join(root, ".trash", "20240102-030408"), 0o755)
	os.Symlink(victim, filepath.Join(root, ".trash", "20240102-030408", "victim"))
	write(filepath.Join(root, ".trash", "20240102-030408", "victim.origin"), filepath.Join(agentDir, "victim")+"\n")
	l.IDs = append(l.IDs, filepath.ToSlash(up), "20240102-030406/../../../outside/victim",
		"20240102-030407/victim", "20240102-030408/victim", filepath.ToSlash(victim))
	return l
}

// Settings is a settings file (config.toml) whose editor and a key skd
// does not know hold Evil.
func Settings(path string) {
	write(path, "editor = "+TOML("vim"+Evil)+"\n"+TOML("k"+Evil)+" = "+TOML(Evil)+"\n")
}
