#!/bin/sh
# Installs the skilldots desktop app and its skd command on macOS:
#
#   curl -fsSL https://skilldots.maybe404.com/install.sh | sh
#
# It downloads the latest release from GitHub, checks its SHA-256, puts
# skilldots.app in /Applications (~/Applications when that is not writable)
# and links /usr/local/bin/skd to the app, asking for your password only
# when that directory needs it. A download by curl is not quarantined, so
# macOS opens the app without the "unverified developer" prompt. Run from
# a terminal, it then opens the app, and says how to start from the
# command line instead.
#
# Environment: SKILLDOTS_APP_DIR and SKILLDOTS_BIN_DIR change the two
# directories; SKILLDOTS_NO_CLI=1 skips the skd command; SKILLDOTS_NO_OPEN=1
# leaves the app closed (it is only opened when stdout is a terminal);
# SKILLDOTS_RELEASE is the download base URL (for testing).
set -eu

REPO="Maybe404/skilldots"
BASE="${SKILLDOTS_RELEASE:-https://github.com/$REPO/releases/latest/download}"
ASSET="skilldots-macos.zip"

say() { printf '%s\n' "$*"; }
fail() { printf 'skilldots: %s\n' "$*" >&2; exit 1; }
# The file a path ends at once every symbolic link is followed, with its
# directory spelled physically, so two names for one file compare equal.
resolve() {
	p=$1
	n=0
	while [ -L "$p" ] && [ "$n" -lt 40 ]; do
		t=$(readlink "$p") || break
		case "$t" in
		/*) p=$t ;;
		*) p=$(dirname "$p")/$t ;;
		esac
		n=$((n + 1))
	done
	d=$(cd -P "$(dirname "$p")" 2>/dev/null && pwd -P) || d=$(dirname "$p")
	printf '%s/%s\n' "$d" "$(basename "$p")"
}

[ "$(uname -s)" = "Darwin" ] || fail "the desktop app is for macOS; on other systems use: go install github.com/$REPO/cmd/skd@latest"

APP_DIR="${SKILLDOTS_APP_DIR:-/Applications}"
if [ -z "${SKILLDOTS_APP_DIR:-}" ] && [ ! -w "$APP_DIR" ]; then
	APP_DIR="$HOME/Applications"
fi
BIN_DIR="${SKILLDOTS_BIN_DIR:-/usr/local/bin}"
APP="$APP_DIR/skilldots.app"
EXE="$APP/Contents/MacOS/skilldots"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT TERM

say "Downloading skilldots…"
curl -fsSL "$BASE/$ASSET" -o "$tmp/$ASSET" || fail "could not download $BASE/$ASSET"
curl -fsSL "$BASE/$ASSET.sha256" -o "$tmp/$ASSET.sha256" || fail "could not download the checksum"
want=$(awk '{print $1}' "$tmp/$ASSET.sha256")
got=$(shasum -a 256 "$tmp/$ASSET" | awk '{print $1}')
[ -n "$want" ] && [ "$want" = "$got" ] || fail "checksum mismatch (expected $want, got $got); nothing was installed"

ditto -x -k "$tmp/$ASSET" "$tmp/unpacked" || fail "could not unpack the download"
[ -x "$tmp/unpacked/skilldots.app/Contents/MacOS/skilldots" ] || fail "the download holds no skilldots.app"

if pgrep -qf "$EXE" 2>/dev/null; then
	say "Quitting the running skilldots…"
	osascript -e 'quit app "skilldots"' >/dev/null 2>&1 || true
	sleep 1
fi
mkdir -p "$APP_DIR"
# Renamed into place (names of this run's own), so a failure never leaves
# half an app behind and nothing left by an earlier run gets in the way.
new="$APP.new.$$"
old="$APP.old.$$"
ditto "$tmp/unpacked/skilldots.app" "$new" || { rm -rf "$new"; fail "could not copy skilldots.app into $APP_DIR; nothing was changed"; }
if [ -e "$APP" ]; then
	mv "$APP" "$old" || { rm -rf "$new"; fail "could not move the old $APP aside; nothing was changed"; }
fi
if ! mv "$new" "$APP"; then
	if [ -e "$old" ]; then
		mv "$old" "$APP" || fail "could not put the new skilldots.app in place, nor the old one back: the old one is at $old, the new one at $new"
	fi
	rm -rf "$new"
	fail "could not put the new skilldots.app in place; the old one was kept"
fi
rm -rf "$old" 2>/dev/null || say "Could not remove the previous version at $old; delete it when you like."
say "Installed $APP"

# skd: replace only a link to some skilldots.app, never another program.
# cli ends up installed, other (another skd is there), or off.
cli=off
link="$BIN_DIR/skd"
if [ "${SKILLDOTS_NO_CLI:-}" = "1" ]; then
	say "Skipped the skd command (SKILLDOTS_NO_CLI=1); turn it on later in skilldots → Settings → Agents (设置 → Agent 接入)."
else
	if [ -e "$link" ] || [ -L "$link" ]; then
		case "$(readlink "$link" 2>/dev/null || true)" in
		*.app/Contents/MacOS/skilldots) ;;
		*)
			say "Left $link alone: it is another program. Remove it, or use that one; skilldots works with either."
			cli=other
			;;
		esac
	fi
	if [ "$cli" = off ]; then
		if mkdir -p "$BIN_DIR" 2>/dev/null && [ -w "$BIN_DIR" ]; then
			if ln -sfh "$EXE" "$link"; then cli=installed; fi
		else
			say "Linking $link needs your password:"
			if sudo mkdir -p "$BIN_DIR" && sudo ln -sfh "$EXE" "$link"; then cli=installed; fi
		fi
		if [ "$cli" = installed ]; then
			say "Installed the skd command: $link"
		else
			say "The skd command was not installed; turn it on in skilldots → Settings → Agents (设置 → Agent 接入)."
		fi
	fi
fi

say ""
# Open the app only for someone watching it happen: stdout is a terminal
# (under `curl | sh` stdin is the pipe, but stdout is still the terminal),
# and SKILLDOTS_NO_OPEN is not 1.
if [ "${SKILLDOTS_NO_OPEN:-}" != "1" ] && [ -t 1 ] && open "$APP" 2>/dev/null; then
	say "Done. skilldots is opening; on first launch it can create your library and teach the agents on this Mac to use skd."
else
	say "Done. Open skilldots from $APP_DIR, or run: open \"$APP\""
	say "On first launch it can create your library and teach the agents on this Mac to use skd."
fi

# The next step for someone who would rather use the command line.
say ""
if [ "$cli" = off ]; then
	say "For the command line, turn on the skd command in skilldots → Settings → Agents (设置 → Agent 接入),"
	say "then run \"skd init\" to create your library and \"skd add owner/repo\" to pick skills from a GitHub repository."
elif [ "$cli" = other ]; then
	say "For the command line: \"skd\" in $BIN_DIR is another program, so it would run instead of skilldots."
	say "Remove it, then turn on the skd command in skilldots → Settings → Agents (设置 → Agent 接入)."
else
	skd=skd
	case ":$PATH:" in
	*":$BIN_DIR:"*) ;;
	*)
		if [ "$cli" = installed ]; then
			skd="$link"
			say "$BIN_DIR is not on your PATH: add it, or run skd as $link."
		fi
		;;
	esac
	# An skd earlier on PATH (say ~/.local/bin or ~/go/bin from go install)
	# would run instead; say which, and leave it be.
	first=$(command -v skd 2>/dev/null || true)
	if [ -n "$first" ] && [ "$(resolve "$first")" != "$(resolve "$link")" ]; then
		skd="$link"
		say "Typing skd runs $first, another skd found first on your PATH, not this skilldots."
		say "Remove it, or put $BIN_DIR before $(dirname "$first") in PATH; until then, run skd as $link."
	fi
	say "Prefer the command line? Set up from the terminal instead of the app:"
	say "  $skd init                  create ~/.skilldots and teach your agents to use skd"
	say "  $skd add owner/repo        pick skills from a GitHub repository (try: $skd add anthropics/skills)"
	say "  $skd                       the full-screen dashboard; \"$skd --help\" lists every command"
fi
