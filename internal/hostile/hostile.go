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
// description), weird (in a directory named with Evil), and two that skd
// must reject: one in a category named with Evil, one whose name has it.
func Upstream(dir string, git Git) {
	write(filepath.Join(dir, "LICENSE"), "MIT License "+Evil+"\n\nPermission is hereby granted\n")
	write(filepath.Join(dir, "skills", "eng", "tdd", "SKILL.md"), skillMD("tdd", "tests first "+Evil))
	write(filepath.Join(dir, "skills", "eng", "review", "SKILL.md"), skillMD("review", Normal))
	write(filepath.Join(dir, "skills", "x"+Evil, "SKILL.md"), skillMD("weird", Evil))
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

// Extras adds what other files of a library and of the machine can
// carry: a history whose every field (and one it does not know) holds
// Evil, trash entries named with it whose recorded origin holds it, and
// in agentDir an unmanaged skill, a copy and a broken link named or
// described with it. home is where the paths in the trash point.
func Extras(root, agentDir, home string) {
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
}

// Settings is a settings file (config.toml) whose editor and a key skd
// does not know hold Evil.
func Settings(path string) {
	write(path, "editor = "+TOML("vim"+Evil)+"\n"+TOML("k"+Evil)+" = "+TOML(Evil)+"\n")
}
