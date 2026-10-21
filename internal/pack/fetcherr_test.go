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
		"fatal: unable to access 'https://x/a.git/': The requested URL returned error: 404":             FetchNotFound,
		"fatal: repository '/no/such/dir' does not exist":                                               FetchNotFound,
		"git@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.": FetchNoAccess,
		"fatal: unable to access 'https://github.com/a/b.git/': Could not resolve host: github.com":     FetchOffline,
		"fatal: unable to access 'https://x/': LibreSSL SSL_connect: SSL_ERROR_SYSCALL in connection":   FetchOffline,
		"fatal: something else entirely": "",
	} {
		if got := fetchKind(msg, false, false); got != want {
			t.Errorf("%q: %q, want %q", msg, got, want)
		}
	}
	if got := fetchKind("warning: Could not find remote branch v9 to clone.\nfatal: Remote branch v9 not found in upstream origin", true, false); got != FetchNoRef {
		t.Errorf("missing ref: %q", got)
	}
}

// Each failure is told for what it is: a repository missing over SSH is
// not sent to the SSH address, an unknown host key is about known_hosts,
// and refused credentials or a 403 are not "cannot find the repository".
func TestFetchKindIsNotMisread(t *testing.T) {
	for _, c := range []struct {
		msg  string
		ssh  bool
		want string
	}{
		{"ERROR: Repository not found.\nfatal: Could not read from remote repository.\n\nPlease make sure you have the correct access rights", true, FetchNoAccess},
		{"fatal: 'git@host:a/b.git' does not appear to be a git repository\nfatal: Could not read from remote repository.", true, FetchNoAccess},
		{"Host key verification failed.\nfatal: Could not read from remote repository.", true, FetchHostKey},
		// A connection that failed over SSH ends in the same last line as a
		// missing repository: it is about the network, said first.
		{"ssh: connect to host 127.0.0.1 port 1: Connection refused\r\nfatal: Could not read from remote repository.\n\nPlease make sure you have the correct access rights\nand the repository exists.", true, FetchOffline},
		{"ssh: Could not resolve hostname nowhere.invalid: nodename nor servname provided, or not known\r\nfatal: Could not read from remote repository.", true, FetchOffline},
		{"ssh: Could not resolve hostname x: Temporary failure in name resolution\r\nfatal: Could not read from remote repository.", true, FetchOffline},
		{"ssh: connect to host github.com port 22: Network is unreachable\r\nfatal: Could not read from remote repository.", true, FetchOffline},
		{"Connection closed by 140.82.112.3 port 22\r\nfatal: Could not read from remote repository.", true, FetchOffline},
		{"kex_exchange_identification: read: Connection reset by peer\r\nfatal: Could not read from remote repository.", true, FetchOffline},
		{"@@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@\nHost key verification failed.", true, FetchHostKey},
		{"remote: Invalid username or token.\nfatal: Authentication failed for 'https://github.com/a/b.git/'", false, FetchAuth},
		{"fatal: unable to access 'https://github.com/a/b.git/': The requested URL returned error: 401", false, FetchAuth},
		{"fatal: unable to access 'https://github.com/a/b.git/': The requested URL returned error: 403", false, FetchForbidden},
		{"remote: Repository not found.\nfatal: repository 'https://github.com/a/b.git/' not found", false, FetchNotFound},
	} {
		if got := fetchKind(c.msg, false, c.ssh); got != c.want {
			t.Errorf("%q (ssh %v): %q, want %q", c.msg, c.ssh, got, c.want)
		}
	}
	for url, ssh := range map[string]bool{"git@github.com:a/b.git": true, "ssh://git@host/a/b": true, "https://github.com/a/b.git": false, "/local/path": false} {
		if sshURL(url) != ssh {
			t.Errorf("sshURL(%q) = %v", url, !ssh)
		}
	}
	for kind, want := range map[string]string{
		FetchNoAccess:  "your SSH key has no access",
		FetchHostKey:   "known_hosts",
		FetchAuth:      "refused the credentials",
		FetchForbidden: "HTTP 403",
	} {
		e := &FetchError{Repo: "a/b", URL: "git@github.com:a/b.git", Kind: kind, Git: errors.New("x")}
		if msg := e.Error(); !strings.Contains(msg, want) || strings.Contains(msg, "use its SSH address") || strings.Contains(msg, "cannot find") {
			t.Errorf("%s: %s", kind, msg)
		}
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
