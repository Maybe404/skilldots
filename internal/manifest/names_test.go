package manifest

import "testing"

func TestCheckName(t *testing.T) {
	for _, ok := range []string{"tdd", "grill-me", "a.b", "A_1", "-x"} {
		if err := CheckName(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", "../../x", "a/b", `a\b`, ".hidden", "a..b", "x y", "/abs", "a\x00b"} {
		if CheckName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCheckCategory(t *testing.T) {
	for _, ok := range []string{"", "a", "a/b", "My Things"} {
		if err := CheckCategory(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"..", "a/../b", "/a", "a/", `a\b`, "."} {
		if CheckCategory(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
