package pack

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// The messages git gives for what people meet are said in plain words,
// with git's own after them.
func TestFetchKind(t *testing.T) {
	for msg, want := range map[string]string{
		"fatal: could not read Username for 'https://github.com': Device not configured":                FetchNotFound,
		"fatal: could not read Username for 'https://github.com': terminal prompts disabled":            FetchNotFound,
		"remote: Repository not found.\nfatal: repository 'https://github.com/a/b.git/' not found":      FetchNotFound,
		"fatal: Authentication failed for 'https://gitlab.com/a/b.git/'":                                FetchNotFound,
		"fatal: unable to access 'https://x/a.git/': The requested URL returned error: 404":             FetchNotFound,
		"fatal: repository '/no/such/dir' does not exist":                                               FetchNotFound,
		"git@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.": FetchNoAccess,
		"fatal: unable to access 'https://github.com/a/b.git/': Could not resolve host: github.com":     FetchOffline,
		"fatal: unable to access 'https://x/': LibreSSL SSL_connect: SSL_ERROR_SYSCALL in connection":   FetchOffline,
		"fatal: something else entirely":                                                                "",
	} {
		if got := fetchKind(msg, false); got != want {
			t.Errorf("%q: %q, want %q", msg, got, want)
		}
	}
	if got := fetchKind("warning: Could not find remote branch v9 to clone.\nfatal: Remote branch v9 not found in upstream origin", true); got != FetchNoRef {
		t.Errorf("missing ref: %q", got)
	}
}

func TestFetchErrorSaysWhatToDo(t *testing.T) {
	gitErr := errors.New("git clone: fatal: could not read Username for 'https://github.com': terminal prompts disabled")
	err := fetchError(Source{Repo: "nobody/none", URL: "https://github.com/nobody/none.git"}, gitErr)
	var fe *FetchError
	if !errors.As(err, &fe) || fe.Kind != FetchNotFound || !errors.Is(err, gitErr) {
		t.Fatalf("%#v", err)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "cannot find the repository nobody/none on GitHub, or it is private") ||
		!strings.Contains(msg, "git@github.com:nobody/none.git") || !strings.Contains(msg, "(git said: "+gitErr.Error()+")") {
		t.Fatalf("message: %s", msg)
	}
	other := errors.New("git clone: fatal: odd")
	if err := fetchError(Source{Repo: "a/b", URL: "https://github.com/a/b.git"}, other); err != other {
		t.Fatalf("an error skd cannot tell is changed: %v", err)
	}
}

// Real git, in English whatever the system's language: a path with no
// repository, and a ref the repository does not have.
func TestFetchErrorsFromGit(t *testing.T) {
	src, up := fetchEnv(t)
	t.Setenv("LANG", "zh_CN.UTF-8")
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	missing := filepath.Join(filepath.Dir(up), "nothing-here")
	_, _, err := Fetch(Source{Repo: "local/missing", URL: missing})
	var fe *FetchError
	if !errors.As(err, &fe) || fe.Kind != FetchNotFound || !strings.HasPrefix(err.Error(), "there is no git repository at "+missing) {
		t.Fatalf("missing repository: %v", err)
	}
	src.Ref = "no-such-tag"
	_, _, err = Fetch(src)
	if !errors.As(err, &fe) || fe.Kind != FetchNoRef || !strings.Contains(err.Error(), `has no branch or tag "no-such-tag"`) {
		t.Fatalf("missing ref: %v", err)
	}
}
