package skillmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetName(t *testing.T) {
	cases := map[string]string{
		"---\nname: up\ndescription: d\n---\nbody name: x\n": "---\nname: new\ndescription: d\n---\nbody name: x\n",
		"---\ndescription: d\n---\nbody\n":                   "---\nname: new\ndescription: d\n---\nbody\n",
		"just text\n":                                        "---\nname: new\n---\njust text\n",
	}
	for in, want := range cases {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(in), 0o644)
		if err := SetName(dir, "new"); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if string(b) != want {
			t.Errorf("in %q:\n got %q\nwant %q", in, b, want)
		}
		if m, _ := Read(dir); m.Name != "new" {
			t.Errorf("Read after SetName: %q", m.Name)
		}
	}
}
