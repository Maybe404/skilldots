package manifest

import (
	"regexp"
	"strings"

	"github.com/Maybe404/skilldots/internal/i18n"
	"github.com/Maybe404/skilldots/internal/safetext"
)

var nameRE = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

// CheckName validates a skill name used as a directory or link name: a
// skill's frontmatter name, `as`, [[local]].name and [[app]].name. Names
// come from untrusted upstream repositories and end up joined onto paths,
// so anything that could escape a directory is rejected.
func CheckName(name string) error {
	switch {
	case name == "":
		return i18n.Errorf("empty name")
	case len(name) > 128:
		return i18n.Errorf("name %q is longer than 128 characters", name)
	case strings.Contains(name, ".."):
		return i18n.Errorf("invalid name %q: must not contain \"..\"", name)
	case !nameRE.MatchString(name):
		return i18n.Errorf("invalid name %q: use letters, digits, '.', '_' and '-' only, not starting with '.'", name)
	}
	return nil
}

// CheckCategory validates a category path ("a/b"); "" is valid. It is an
// upstream directory path that becomes one in vendor/ and is shown in
// listings, so control characters, bidi controls and invalid UTF-8 are
// refused along with what could escape a directory.
func CheckCategory(cat string) error {
	if cat == "" {
		return nil
	}
	if safetext.Has(cat) {
		return i18n.Errorf("invalid category %q: control characters", cat)
	}
	for _, seg := range strings.Split(cat, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, "\\\x00") {
			return i18n.Errorf("invalid category %q", cat)
		}
	}
	return nil
}
