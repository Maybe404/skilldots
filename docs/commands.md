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
  - `skd outdated` does the same with `{checked, packs, failed, updates}`; each pack has `repo, commit, latest, changed, moved, new, gone, pending, checked`, and `shelved` when an update of one of its skills waits for `skd resolve` (those skills are in `changed` too).
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

Each unmanaged entry says what `skd adopt` would do with it, or why it is left to you (say, a name the library already uses twice over); the last line counts those. It never fetches, so a copy is compared with upstream only by `adopt`.

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

Then it re-vendors the selected skills, reports, and records the update in the history. `--commit` commits and pushes the library when something changed (it refuses while a skill is being resolved).

A skill you edited is merged, not replaced, the way `git pull` merges:

- The base is the upstream version your edits were made on: the one whose content hash `skills.lock` records. skd finds it in the library's upstream snapshots (below), else in the library's own git history of the skill's directory; with neither, there is no base and every file both sides changed is a conflict.
- File by file: a file only one side changed takes that side; text both changed goes through `git merge-file`; a file one side removed and the other changed, a file both added differently, and a non-text file both changed are conflicts.
- Merged cleanly: the copy as it was goes to the trash (reason `merged`), the merge is written in its place, and the report lists the skill in `changed` and `merged`. The skill stays edited, so the next update merges again.
- Conflicts: the update of that skill is **shelved**. Its directory and its lock hash stay as they were; `skills.lock` records `shelved` (the upstream commit), `upstream` (that version's hash) and `conflicts` (the files); older skd versions ignore these. The rest of the pack is updated and its commit moves on. The report lists the skill under `shelved`, with the files and the command that resolves it. `skd list` marks it `shelved`, and `skd outdated` counts it as an update (`shelved` in `--json`).
- A later update merges a shelved skill again against the newest upstream version; it clears the shelf when upstream went back to the base or the merge is clean. A skill being resolved is left alone.

Every update also records the upstream version vendored, as a commit on `refs/skilldots/upstream/<pack>` in the library's repository (see [design](design.md)): the next merge's base, and what `history --diff` reads. In a library that is not a git repository nothing is recorded, and an update that merged or shelved a skill says so.

### `skd resolve [skill] [--continue | --abort]`

Resolves a shelved update, as you would finish a git merge.

- `skd resolve` lists the shelved skills, with their conflicted files and whether one is being resolved.
- `skd resolve <skill>` works the merge out again against the newest upstream version (merging cleanly, if your edits changed meanwhile, finishes it). It moves the skill's directory to the trash (`merged`), writes the merge into it with git's conflict markers (`<<<<<<< local`, `=======`, `>>>>>>> upstream <commit>`), and puts the conflicted files into the library's git index as unmerged entries (stage 1 the base, 2 local, 3 upstream). So `git status` shows them as both modified (or deleted by one side), `git diff` shows the combined diff, and `git checkout --ours|--theirs -- <file>` takes a side.
- Files that cannot hold markers: a non-text file changed on both sides keeps yours, with upstream's beside it as `<file>.upstream`; a file you removed and upstream changed stays removed, with upstream's as `<file>.upstream`; a file upstream removed and you changed stays, marked by an empty `<file>.upstream-deleted`. Keep one version and delete the other file.
- `--continue [skill]` refuses (non-zero exit; `--json`: `{skill, unresolved}`) while a file of the skill holds a line opening or closing a conflict (`<<<<<<<`, `>>>>>>>`), a conflicted file still holds `=======` or `|||||||` alone on a line, or a `.upstream`/`.upstream-deleted` file is left. Otherwise it runs `git add` on the skill's directory, records upstream's version as merged in `skills.lock`, links the skill again, and adds a history record listing it under `merged`. `skd sync` commits.
- `--abort [skill]` moves the files as they are to the trash, puts back the copy from before the resolve, restores the index entries the conflicts replaced, and leaves the update shelved.
- Without a skill, `--continue` and `--abort` act on the one skill being resolved.

While a skill is being resolved its files hold the markers, and the agents linked to it read them. `skd sync` and `update --commit` refuse to commit meanwhile.

Saving a file in the desktop app's editor stages it (`git add`), like saving in an editor and adding in git; `skd sync` (or `update --commit`) commits. `skd edit` only opens an editor and stages nothing.

### `skd history [pack] [-n N] [--diff [n] [--skill x]]`

What each update changed: when, old → new commit, the skills changed / merged / shelved / added / moved / gone / new upstream, and the subjects of up to 20 upstream commits that touched them.

- `--diff` prints what upstream changed in one record as a unified diff, coloured like git's, skill by skill: the newest record (of the pack, when one is named), or the n-th newest (`skd history owner/repo --diff 2`); `--skill` keeps one skill. It compares the upstream snapshots the record names, offline. A record from before skd kept snapshots is read from the pack's cache clone, fetching the record's commits by hash when the clone lacks them (that needs the network, and fails, saying so, without it). `--json` prints `{record, skills, source}`.

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

Here and in every command that takes a pack (`update`, `list`, `enable` and the rest), a pack is named by `owner/repo`, by the URL or local path it was added from (`file://` URLs included), or by the last part of its repository name when only one pack has it.

## Taking over and cleaning up

### `skd adopt [-n] [--yes]`

Takes over unmanaged entries found by `skd scan`: subscribes to their repository, registers apps, copies the rest in as your own, and enables each for the agents it was found in.

- A copy in the own directory of an agent that reads `~/.agents/skills`, of a skill the library already links into `agents`, is redundant: when it is the same as the library's, it goes to the trash (that agent keeps reading the library's copy through `agents`); when it differs, it is left in place with a note.
- A copy whose repository skd knows is compared with upstream first (one fetch per repository). If you changed it, it is copied in as your own skill rather than replaced by upstream's; the proposal names the repository and the `skd add` that would follow upstream instead (`Upstream` and `Differs` in `--json`).
- A copy whose content is exactly a library skill's, under whatever name, is taken for that skill: it is installed for the agent and the copy goes to the trash.
- A copy taken in as your own is named by its `SKILL.md`; when the library already uses that name, by its directory (and its `SKILL.md` is renamed to match), as the dry run says (`As` in `--json`). With both names taken it is not adopted: rename it first.
- What it leaves to you is listed, with the reason, before anything is done, with `--yes` too.

### `skd trash [restore <id|skill|owner/repo> | empty]`

Lists what skd set aside, with the skills in each entry and why (`!` marks an entry skd will not restore, with the reason under the table); `restore` takes an ID, a skill name or, for a purged pack, `owner/repo` (several matches are listed, never guessed) and puts the entry back, undoing `remove` and `purge` entirely (manifest, lock and links); `empty` deletes everything for good, interactively, and only for a person at a terminal ([safety.md](safety.md#the-trash)).

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
