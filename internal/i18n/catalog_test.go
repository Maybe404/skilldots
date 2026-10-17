package i18n_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Maybe404/skilldots/internal/i18n"
)

// wrappers are functions that translate a format they are given, by
// name (a method matches by its name alone) and the position of that
// argument. A function that shows a message and takes it as an argument
// belongs here, so its literals are held to the catalogue too.
var wrappers = map[string][]int{
	"say": {0}, "printf": {0}, "joinOr": {1}, "confirm": {0}, "printGroup": {4},
	"pickEntry": {0}, "opt": {0, 1}, "key": {1},
	"logf": {1}, // app.Session.logf(level, format, ...)
}

// variadic are functions whose arguments from the position on are all
// messages (the column headers of newTable).
var variadic = map[string]int{"newTable": 0}

// Calls of this package whose argument at the position is a message.
var own = map[string][]int{
	"T": {0}, "M": {0}, "Sprintf": {0}, "Errorf": {0}, "New": {0}, "Fprintf": {1}, "Count": {1, 2},
}

var verbRE = regexp.MustCompile(`%[-+# 0-9.*]*(?:\[[0-9]+\])?[a-zA-Z%]`)
var letterRE = regexp.MustCompile(`[A-Za-z]`)

// constString evaluates e when it is a string literal or literals joined
// with "+".
func constString(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(x.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return "", false
		}
		l, ok1 := constString(x.X)
		r, ok2 := constString(x.Y)
		return l + r, ok1 && ok2
	case *ast.ParenExpr:
		return constString(x.X)
	}
	return "", false
}

// needsTranslation reports whether a message has words: one that is only
// verbs and punctuation reads the same in every language.
func needsTranslation(msg string) bool {
	return letterRE.MatchString(verbRE.ReplaceAllString(msg, ""))
}

type use struct {
	msg      string
	pos      token.Position
	optional bool // the singular of Count: Chinese has no plural
}

// uses finds every message the source passes to this package.
func uses(t *testing.T) (found []use, rawErrors []string) {
	t.Helper()
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	err := filepath.Walk(filepath.Join(root, "internal"), func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		if strings.Contains(filepath.ToSlash(p), "internal/i18n/") || strings.Contains(filepath.ToSlash(p), "internal/hostile/") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, p, src, parser.ParseComments)
		if err != nil {
			return err
		}
		lines := strings.Split(string(src), "\n")
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var pkg, name string
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fn.Sel.Name
				if id, ok := fn.X.(*ast.Ident); ok {
					pkg = id.Name
				}
			case *ast.Ident:
				name = fn.Name
			}
			var at []int
			switch {
			case pkg == "i18n":
				at = own[name]
			case pkg == "fmt" && name == "Errorf", pkg == "errors" && name == "New":
				if len(call.Args) > 0 {
					if s, ok := constString(call.Args[0]); ok && needsTranslation(s) {
						pos := fset.Position(call.Pos())
						if !strings.Contains(lines[pos.Line-1], "i18n:raw") {
							rawErrors = append(rawErrors, fmt.Sprintf("%s: %s(%q)", pos, pkg+"."+name, s))
						}
					}
				}
			default:
				if idx, ok := wrappers[name]; ok && pkg != "i18n" {
					at = idx
				} else if from, ok := variadic[name]; ok && pkg != "i18n" {
					for i := from; i < len(call.Args); i++ {
						at = append(at, i)
					}
				}
			}
			for k, i := range at {
				if i >= len(call.Args) {
					continue
				}
				if s, ok := constString(call.Args[i]); ok {
					found = append(found, use{msg: s, pos: fset.Position(call.Args[i].Pos()), optional: pkg == "i18n" && name == "Count" && k == 0})
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found, rawErrors
}

var verbPartsRE = regexp.MustCompile(`%[-+# 0-9.*]*(\[([0-9]+)\])?([a-zA-Z%])`)

// argVerbs maps each argument position (from 1) to the verb that formats
// it, following %[n]s indexes the way fmt does.
func argVerbs(s string) map[int]string {
	out := map[int]string{}
	next := 1
	for _, m := range verbPartsRE.FindAllStringSubmatch(s, -1) {
		if m[3] == "%" {
			continue
		}
		pos := next
		if m[2] != "" {
			pos, _ = strconv.Atoi(m[2])
		}
		out[pos] = m[3]
		next = pos + 1
	}
	return out
}

func verbs(s string) string {
	var v []string
	for _, m := range verbRE.FindAllString(s, -1) {
		if m != "%%" {
			v = append(v, m[len(m)-1:])
		}
	}
	sort.Strings(v)
	return strings.Join(v, "")
}

// edges are the line breaks at the start and the end of s: a translation
// keeps them (spaces next to a Chinese character are the translator's).
func edges(s string) (string, string) {
	lead := s[:len(s)-len(strings.TrimLeft(s, " \t\r\n"))]
	trail := s[len(strings.TrimRight(s, " \t\r\n")):]
	only := func(x string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' {
				return r
			}
			return -1
		}, x)
	}
	return only(lead), only(trail)
}

// Every message the source shows has a Chinese translation, and every
// translation is for a message the source shows: add the English where
// it is used and the Chinese in the catalogue, together.
func TestEveryMessageIsTranslated(t *testing.T) {
	cat := i18n.Catalog()
	found, _ := uses(t)
	if len(found) < 10 {
		t.Fatalf("found only %d messages: the scan is broken", len(found))
	}
	used := map[string]bool{}
	var skeleton strings.Builder
	seen := map[string]bool{}
	for _, u := range found {
		used[u.msg] = true
		if u.optional || !needsTranslation(u.msg) {
			continue
		}
		if _, ok := cat[u.msg]; !ok {
			t.Errorf("%s: no Chinese translation of %q", u.pos, u.msg)
			if !seen[u.msg] {
				seen[u.msg] = true
				fmt.Fprintf(&skeleton, "# %s\n@ %s\n= \n\n", u.pos, i18n.Escape(u.msg))
			}
		}
	}
	// I18N_SKELETON=file writes the missing entries, ready to translate.
	if path := os.Getenv("I18N_SKELETON"); path != "" && skeleton.Len() > 0 {
		os.WriteFile(path, []byte(skeleton.String()), 0o644)
	}
	for k := range cat {
		if !used[k] {
			t.Errorf("translation of a message the source does not use (stale or misspelt): %q", k)
		}
	}
}

// A translation keeps the values of its message (same verbs, any order,
// with %[n]s to reorder) and its leading and trailing line breaks.
func TestTranslationsKeepTheirValues(t *testing.T) {
	for k, v := range i18n.Catalog() {
		if strings.TrimSpace(v) == "" {
			t.Errorf("empty translation of %q", k)
			continue
		}
		if verbs(k) != verbs(v) {
			t.Errorf("verbs differ:\n  %q (%s)\n  %q (%s)", k, verbs(k), v, verbs(v))
		}
		// Each value is formatted by the same verb as in the message: a
		// translation that swaps two values needs %[n]s to say so.
		kv, vv := argVerbs(k), argVerbs(v)
		for pos, verb := range kv {
			if vv[pos] != verb {
				t.Errorf("argument %d is formatted by %%%s in the message but %%%s in the translation:\n  %q\n  %q", pos, verb, vv[pos], k, v)
				break
			}
		}
		kl, kt := edges(k)
		vl, vt := edges(v)
		if kl != vl || kt != vt {
			t.Errorf("leading/trailing line breaks differ:\n  %q\n  %q", k, v)
		}
	}
}

// Errors and messages are made with i18n.Errorf / i18n.New, so that they
// are translated; fmt.Errorf and errors.New with words in them are not.
func TestNoUntranslatedErrors(t *testing.T) {
	_, raw := uses(t)
	for _, r := range raw {
		t.Errorf("use i18n.Errorf / i18n.New (or mark the line // i18n:raw if it is not shown): %s", r)
	}
}
