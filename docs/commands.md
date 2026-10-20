# Command reference

Every `skd` command in full. The [README](../README.md#commands) has a one-line summary of each; what skd will and will not delete is in [safety.md](safety.md).

## Flags every command takes

- `--json`: structured output for scripts and agents (see [below](#what---json-prints)).
- `--yes`: skip prompts.
- `--lang en|zh|auto`: the language of skd's own text ([README → Language](../README.md#language)).
- `--theme light|dark|auto`: colours. The default is `auto`; skd also reads `$SKILLDOTS_THEME`, then `skd config set theme`. A value skd does not know falls back to `auto` with a warning. `NO_COLOR` turns colour off.

## What `--json` prints

- stdout carries exactly one JSON document. Progress and warnings go to stderr.
- A failing command prints nothing on stdout and exits non-zero. Three commands print their document and still exit non-zero:
  - `skd update` reports `{packs, failed, apply, committed, history}`. It exits non-zero when any pack failed, and also, with an `error` field, when the update was saved but linking or `--commit` then failed.
  - `skd outdated` does the same with `{checked, packs, failed, updates}`; each pack has `repo, commit, latest, changed, moved, new, gone, pending, checked`.
  - `skd apply` exits non-zero when a planned change was not made because its agent directory no longer led where it did, or its source went through a symbolic link inside the library (`refused`). The last line of the plain output then reads `applied · … · N not changed` instead of a tick.
- Problems confined to one skill (an unknown target, a name clash, a pack not fetched yet) are warnings, not errors: the rest of the library keeps working.
- `--json` is English whatever the language, byte for byte what `--lang en` gives ([README → Language](../README.md#language)).
- Field values are data as found. Every control character in them (DEL, C1 and bidi controls included) is written as a `\u` escape, so the document decodes to the same values and is harmless on a terminal; a program that shows them to a person cleans them itself ([safety.md](safety.md#what-skd-prints)).

## Setting up a library

### `skd init [dir]`

Creates a library in `dir` (default `~/.skilldots`, or `$SKILLDOTS_LIBRARY`).

- The library is its own git repository, even inside another repo; `private/` is a separate one.
- Default targets of a new library: `agents` (`~/.agents/skills`, which nearly every agent reads), plus `claude` when Claude Code is installed, because Claude Code does not read `~/.agents/skills`. On a machine without Claude Code, no `~/.claude` is made.
- It installs the [`skilldots` skill](../skills/skilldots/SKILL.md) for every agent on the machine: through `~/.agents/skills` and the own directory of each installed agent that does not read it ([README → Built for agents](../README.md#built-for-agents)).

### `skd clone <git-url> [dir]`

Sets up a machine from a library you pushed elsewhere.

- It lists the library's custom targets first and, at a terminal, asks whether to approve each ([safety.md → Custom targets](safety.md#custom-targets)).
- A `private` that is a symbolic link in the cloned library is left as it is: no git repository is set up through it, and the skills under it are skipped. `skd init --from` refuses one.

### `skd use <dir>`

Points skd at an existing library, from any directory. After moving a library: `skd use <new dir>`, then `skd apply` ([safety.md → Moved or copied libraries](safety.md#moved-or-copied-libraries)).

### `skd sync [--remote url] [-m msg]`

Commits the library and pushes it: your backup.

## Adding skills

### `skd show <owner/repo | url>`

Lists what a repository offers, without installing anything. `skd show owner/repo --json` lets an agent inspect a repository before `skd add --skill …`.

### `skd add <source>[@ref]`

Fetches a repository and lets you choose skills interactively, or with flags:

- skd never asks for a username or password: a private repository over HTTPS needs a git credential helper, or use its SSH address. When a repository cannot be fetched, skd says why (not found or private, no SSH access, no such ref, network), with git's own words after it.

- `--skill x`, `--category c`, `--all`. `--all` follows everything; names given with it are only checked.
- On a pack that follows `"*"`, `--skill x` takes `x` out of `skip`. If a wildcard such as `eng/*` excludes it, the command fails and tells you to edit `skills.toml`.
- With no source at a terminal, it asks for one.

The source can be:

- `owner/repo` or a git URL.
- A whole install line, quoted: `npx skills add owner/repo --skill x`, and the same with bunx, pnpx, `pnpm dlx`, `yarn dlx`, `npm exec`, `npx add-skill` or `skd add`.
- A skills.sh skill page.
- A GitHub link to a skill directory (`/tree/<ref>/<dir>`) or its `SKILL.md` (`/blob/<ref>/…`).

For an install line, a skills.sh page or a GitHub link, the repository is taken from it and the skills it names are ticked in the picker; without a terminal they are the selection.

Rules for pasted text:

- It is only parsed, never run.
- Unquoted, only an install command may span several arguments. A local path with a space needs quotes too.
- A source or `@ref` starting with `-` is refused.
- The `<ref>` in a GitHub link is where the page was browsed and is not pinned. Append `@ref` to pin.

## Looking around

### `skd list [pack] [--all] [--flat] [--agent x]`

An overview, one line per repository. Name a pack to expand it; `--flat` lists skills one per line; `--agent x` shows what one agent receives. `skd list` and the menu also show the last `skd outdated` result (`↑ 2 updates available · checked 2h ago`, stale after a day) without fetching.

### `skd path <skill>`

Prints where a skill's files live, for editing.

### `skd scan`

Inventories every agent skill directory: managed entries, copies, links and broken links. It also lists links into the library recorded for a custom target no longer in use (`left`), and leaves them in place, since they may be your own.

## Linking

### `skd apply [-n] [--force]`

Links and unlinks to match the manifest.

- `-n` previews.
- `--force` replaces entries in the way of a wanted skill; they are moved to the trash.
- An entry no longer wanted is removed only when skd can tell it made it. Otherwise it stays, and the plan says so.
- skd does not create the skills directory of a built-in agent that is not installed on the machine (the shared `~/.agents/skills` excepted): apply skips that agent and says so.

### `skd enable <skill|pack> [--agent a,b]`

Links into agents: the default set when `--agent` is omitted.

### `skd disable <skill|pack> [--agent a,b]`

Unlinks from agents: all of them when `--agent` is omitted. The library keeps the skill.

### `skd targets`

Lists the known agent directories, which of them exist on this machine, and the custom targets of `skills.toml` that are not used, with why.

- `skd targets trust <name>` approves a custom target; only a person at a terminal can ([safety.md → Custom targets](safety.md#custom-targets)). `skd targets untrust <name>` withdraws that.
- `skd targets default <agent>...` sets `targets.default`: the agents a skill without its own list is linked into.
- `skd targets pause <agent>...` pauses agents: skd takes out the links it made there and links nothing until they are resumed. Every skill keeps them selected, so `skd targets resume <agent>...` puts back exactly what was there. Kept in `skills.toml` as `targets.disabled`, so it reaches every machine using the library.

## Updating

### `skd outdated [pack…]`

Checks upstream without changing anything. Per pack it lists:

- the selected skills that changed;
- skills new upstream since the last update (or since `add`, for a pack never updated);
- skills gone upstream;
- skills that only moved to another directory, as `moved`. A move is not an update: a pack with nothing but moves is not counted, and `U` in the dashboard skips it, though `u` or `skd update` on it still renames the vendored directory to the new path.

It exits non-zero if a pack could not be fetched.

### `skd update [pack…] [--commit]`

At a terminal:

1. It checks upstream and lists each pack's changes (changed / new / gone / moved), every pack ticked.
2. Only the ticked packs are updated; ctrl+c cancels.
3. Named packs with nothing new upstream are offered an "update anyway", which re-vendors missing skills and re-links (as `u` in the dashboard). With no pack named, it points to `skd apply` instead.

With `--yes`, `--json` or no terminal, it updates straight away: every pack when none are named.

Then it re-vendors the selected skills, reports, and records the update in the history. `--commit` commits and pushes the library when something changed.

### `skd history [pack] [-n N]`

What each update changed: when, old → new commit, the skills changed / added / moved / gone / new upstream, and the subjects of up to 20 upstream commits that touched them.

- An update whose linking failed says so until an apply links every skill of that pack. One skill still skipped (say, because something is in the way) keeps the note.
- It is read from `skills.history.jsonl` in the library, which is committed and pushed with it, so it works offline and on another machine after a clone.

## Changing skills

### `skd new <name> [--private]`

Scaffolds a skill of your own and links it. The name follows the [skill name rule](../README.md#how-skills-are-found-in-a-repository).

### `skd fork <skill> [--private] [--as name]`

Copies a vendored skill into `skills/` (or `private/`) so you can edit it; upstream no longer touches it. The vendored original is deselected, and the fork's frontmatter is renamed to the fork's name, so the two never clash.

### `skd edit <skill> [--fork [--private] [--as name] | --force]`

Opens a skill's directory in your editor. The editor is, first match wins: the `editor` setting, `$VISUAL`, `$EDITOR`, then VS Code, Cursor, Zed or Sublime Text (command or macOS app), then the system opener.

- A vendored or app skill is replaced on update, so at a terminal skd offers to fork it first. Without a terminal it refuses unless `--fork` (fork, then open the copy) or `--force` (open the files anyway) is given.
- A terminal editor (vim, nano) needs a terminal.
- A command skd does not know as a window editor (code, cursor, zed, subl and the like) is taken for a terminal editor. If yours opens a window, `skd config set editor-kind window` says so (and `terminal` the other way).
- When the editor cannot run (not installed, or a terminal editor without a terminal), `--fork` fails before forking anything.

### `skd remove <skill>`

Unlinks a skill everywhere and moves its directory to the library trash; `skd trash restore <skill>` undoes it: the files, the skill's place in `skills.toml` and `skills.lock`, and its links.

- A vendored skill is deselected and leaves `vendor/`. Removing a pack's last skill drops the pack.
- Your own skill leaves `skills/` or `private/`.
- An app's skill is only unlinked.

### `skd purge <pack>`

Deletes a pack from the manifest, the lock, `vendor/` and every agent directory. What it takes out goes to the trash.

## Taking over and cleaning up

### `skd adopt [-n] [--yes]`

Takes over unmanaged entries found by `skd scan`: subscribes to their repository, registers apps, copies the rest in as your own, and enables each for the agents it was found in.

- A copy whose repository skd knows is compared with upstream first (one fetch per repository). If you changed it, it is copied in as your own skill rather than replaced by upstream's; the proposal names the repository and the `skd add` that would follow upstream instead (`Upstream` and `Differs` in `--json`).

### `skd trash [restore <id|skill> | empty]`

Lists what skd set aside, with the skills in each entry and why; `restore` takes an ID or a skill name (several matches are listed, never guessed) and puts the entry back, undoing `remove` and `purge` entirely (manifest, lock and links); `empty` deletes everything for good, interactively, and only for a person at a terminal ([safety.md](safety.md#the-trash)).

## Settings

### `skd config list | get <key> | set <key> [value]`

Settings, per user and machine:

| key | value |
|---|---|
| `editor` | the editor command |
| `editor-kind` | `auto`, `window` or `terminal`: whether that command opens a window or needs the terminal |
| `theme` | `light`, `dark` or `auto` |
| `lang` | `auto`, `en` or `zh`: the language of skd's own text ([README → Language](../README.md#language)) |
| `library` | shown here, set by `skd use` |

- `set` with no value unsets the key.
- `set` rewrites only that setting's line, so comments in the file stay, a comment at the end of that line too. Unsetting removes the line with its comment. A value spanning several lines makes skd write the file again without comments. New lines end like the file's (LF or CRLF).
- A settings file that is not valid TOML is reported once, with its path and the parse error: as the error of a command that needs it (finding the remembered library, `config`, `use`, and `init` and `clone`, which check before creating anything), or as one warning after a command that ran without its settings.

## The dashboard

### `skd [--simple]`

With no arguments, in a terminal at least 90 columns wide: the full-screen dashboard ([README → Dashboard](../README.md#dashboard)). `--simple`, a narrower terminal or `TERM=dumb` give a question-and-answer menu instead.
