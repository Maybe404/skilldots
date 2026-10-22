# Command reference

Every `skd` command in full. The [README](../README.md#commands) has a one-line summary of each; what skd will and will not delete is in [safety.md](safety.md).

## Flags every command takes

- `--json`: structured output for scripts and agents (see [below](#what---json-prints)).
- `--yes`: skip prompts.
- `--lang en|zh|auto`: the language of skd's own text ([README → Language](../README.md#language)).
- `--theme light|dark|auto`: colours. The default is `auto`; skd also reads `$SKILLDOTS_THEME`, then `skd config set theme`. A value skd does not know falls back to `auto` with a warning. `NO_COLOR` turns colour off.

## What `--json` prints

- stdout carries exactly one JSON document. Progress and warnings go to stderr.
- A failing command prints nothing on stdout and exits non-zero. These print their document and still exit non-zero:
  - `skd update` reports `{packs, failed, apply, committed, history}`. It exits non-zero when any pack failed, and also, with an `error` field, when the update was saved but linking or `--commit` then failed.
  - `skd outdated` does the same with `{checked, packs, failed, updates}`; each pack has `repo, commit, latest, changed, moved, new, gone, pending, checked`, and `shelved` when an update of one of its skills waits for `skd resolve` (those skills are in `changed` too).
  - `skd apply` exits non-zero when a planned change was not made because its agent directory no longer led where it did, or its source went through a symbolic link inside the library (`refused`). The last line of the plain output then reads `applied · … · N not changed` instead of a tick.
  - `skd sync` when the remote and the library both changed the same lines: the document lists them in `conflicts` (the sync is shelved, nothing was merged or pushed).
  - `skd sync --continue` and `skd resolve --continue` while a conflict is left: `{unresolved}` (and `skill` for resolve).
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

- The repository must be a library: `skills.toml` at its top. `dir` must not exist yet.
- A repository without one, such as your own repository of skills, is not kept: the clone is removed, nothing is recorded, skd exits non-zero without a `✓`, and the error says it is a repository of skills and names the command that uses it as a source instead: `skd add <git-url> --all` ([`skd add`](#skd-add-sourceref)). The desktop app's "Import from GitHub" says the same.
- It lists the library's custom targets first and, at a terminal, asks whether to approve each ([safety.md → Custom targets](safety.md#custom-targets)).
- A `private` that is a symbolic link in the cloned library is left as it is: no git repository is set up through it, and the skills under it are skipped. `skd init --from` refuses one.

### `skd use <dir>`

Points skd at an existing library, from any directory. After moving a library: `skd use <new dir>`, then `skd apply` ([safety.md → Moved or copied libraries](safety.md#moved-or-copied-libraries)).

### `skd sync [--remote url] [-m msg] [--resolve | --continue | --abort]`

Commits the library and pushes it: your backup, and how other machines get your changes.

- When the branch has an upstream, sync first fetches it and merges what it has that the library has not (`git merge --no-edit`, never a rebase), links what the merge brought in (as `skd clone` does), then pushes, with the upstream snapshots (`refs/skilldots/*`, forced).
- When both sides changed the same lines the merge is aborted: the library stays exactly as it was, nothing is pushed, the conflicting files are listed (`conflicts` in `--json`), and sync exits non-zero. The sync is shelved.
- `--resolve` redoes the merge and leaves git's conflict state in the library: conflict markers in the files and unmerged index entries, so `git status`, `git diff` and `git checkout --ours|--theirs` work. Agents linked to skills among those files read the markers until it is finished. Commits wait meanwhile.
- `--continue` refuses (non-zero; `--json`: `{unresolved}`) while a conflicted file holds a marker or stays unmerged; otherwise it adds the files, commits the merge, links what it brought in and pushes. `--abort` runs `git merge --abort`.
- `private/` is a separate repository and is never pushed. `--json` prints `{committed, pushed, remote, pulled, conflicts, apply, privateUncommitted}`.

## Adding skills

### `skd show <owner/repo | url>`

Lists what a repository offers, without installing anything. `skd show owner/repo --json` lets an agent inspect a repository before `skd add --skill …`.

### `skd add <source>[@ref]`

Fetches a repository and lets you choose skills interactively, or with flags:

- skd never asks for a username or password: a private repository over HTTPS needs a git credential helper, or use its SSH address. When a repository cannot be fetched, skd says why (not found or private, no SSH access, no such ref, network), with git's own words after it.

- `--skill x`, `--category c`, `--all`. `--all` follows everything; names given with it are only checked.
- `--own` marks the repository as your own (`own = true`, see [Your own repository](#your-own-repository)); it may then have no skill yet, and is followed whole. Without it, at a terminal, skd asks when GitHub says you administer the repository (gh installed and logged in). No license warning is given for your own repository.
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
- skills upstream changed to exactly what you have, as `landed` (your change landed there, after `skd contribute`). Not an update either; `skd update` moves the lock to that version.

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
- Merged cleanly: the copy as it was goes to the trash (reason `merged`), the merge is written in its place. The update's `--json` report lists the skill in `Changed` and `Merged`; the printed report and the history list it once, under merged (a history record's `changed` leaves it out). The skill stays edited, so the next update merges again.
- Conflicts: the update of that skill is **shelved**. Its directory and its lock hash stay as they were; `skills.lock` records `shelved` (the upstream commit), `upstream` (that version's hash) and `conflicts` (the files); older skd versions ignore these. The rest of the pack is updated and its commit moves on. The report lists the skill under `shelved`, with the files and the command that resolves it. `skd list` marks it `shelved`, and `skd outdated` counts it as an update (`shelved` in `--json`).
- A later update merges a shelved skill again against the newest upstream version; it clears the shelf when upstream went back to the base or the merge is clean. A skill being resolved is left alone.
- Upstream's new version is exactly your copy (what you contributed came back, or the same edit was made in both places): the skill **landed**. The lock moves to that version, nothing is merged or set aside in the trash, and the report lists it under `Landed` (`landed` in the history), not under changed or merged. The skill is no longer edited.

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

- `--diff` prints what upstream changed in one record as a unified diff, coloured like git's, skill by skill: the newest record (of the pack, when one is named), or the n-th newest: `skd history owner/repo --diff 2`, or `--diff=2`; a lone number with no pack (`skd history --diff 2`) counts every pack's records. `--skill` keeps one skill. The record `skd resolve --continue` writes goes from the upstream version the local changes were made on to the one merged in, so its diff is what the merge brought in. It compares the upstream snapshots the record names, offline. A record from before skd kept snapshots is read from the pack's cache clone, fetching the record's commits by hash when the clone lacks them (that needs the network, and fails, saying so, without it). `--json` prints `{record, skills, source}`.

- An update whose linking failed says so until an apply links every skill of that pack. One skill still skipped (say, because something is in the way) keeps the note.
- It is read from `skills.history.jsonl` in the library, which is committed and pushed with it, so it works offline and on another machine after a clone.

## Your edits and contributing them

### `skd diff <skill>`

Your changes to a vendored skill: its files against the upstream version they are based on (the one `skills.lock` records, found in the library's upstream snapshots or its git history, as an update's merge finds its base), as a unified diff. When that version is not found it says so and shows nothing. `--json` prints the file/hunk document (`base` false when the version was not found). `skd list` marks a vendored skill you changed with `✎` (`Edited` in `--json`).

### `skd contribute [skill | pack] [--pr | --push] [-m msg] [--dry-run] [--yes] [--refresh]`

Sends your changes to a vendored skill upstream. Nothing is ever sent unless you run this; without a terminal it needs `--yes`.

- Target: the pack's repository at its ref (the default branch when none; a ref that is not a branch is refused). The skill's place there comes from scanning upstream.
- The skill must be edited and based on upstream's current version, not shelved: otherwise "skd update first" (your own repository pulls instead, below).
- The change is made in a clone of its own in the cache (`<cache>/<repo>.contrib`, never the shallow clone updates use, never the library): reset to upstream's tip, the skill's files mirrored over its directory there (deletions too), committed with your git identity and `-m` (default `Update <skill>` and a line `Edited with skilldots`).
- `--push` pushes to the pack's branch when you may (GitHub: asked through gh; elsewhere `git push --dry-run`); a rejected push (a protected branch, upstream moved on) stops with a pointer to `--pr`.
- `--pr` (GitHub only, needs gh logged in) pushes a branch `skilldots/<skill>-<yyyymmdd-hhmm>` to the repository when you may push there, else to your fork (gh forks it, or reuses your fork), and opens the pull request with `gh pr create`. Elsewhere than GitHub: push when you may, or send a patch (`git format-patch`).
- `--dry-run` (or no mode) prints the repository, the branch, what is possible and why not, and the diff; it sends nothing (it fetches upstream).
- The contribution is recorded in `skills.lock` (`contribution`: kind, url, number, branch, state, at) until an update brings upstream to your version (the skill [landed](#skd-update-pack---commit)). `skd contribute` lists the records; `--refresh` (and `skd outdated`, best effort) asks GitHub for the pull requests' state.
- `--json` prints `{repo, mode, branch, url, number, pushed, skills, pulled, pulledBack, pullBackError}` (the options document with `--dry-run`, which also has `own`, `pull`, `skills`, `drafts` and `paths`).

`skd contribute <pack>` sends every skill of the pack you changed and every skill you made in it (`skd new --in`), in one commit (default message `Add c; update a, b`) and, with `--pr`, one pull request from `skilldots/<repo>-<time>`. A name that is both a pack and a skill is refused, with the two spellings that tell them apart (`owner/repo` for the pack, `owner/repo:skill` for the skill).

For [your own repository](#your-own-repository), contribute also pulls, as `git pull` and `git push` would:

- **Before:** when the repository moved on (`pull` in the options), the pack is updated first, merging as `skd update` does. If that shelves one of the skills being sent, nothing is sent: `skd resolve`, then contribute again.
- **After a push:** the pack is updated again, so what was pushed is the version the lock records: the skills land, are no longer marked as changed, and no later update reports them. If that update fails, the push stands and `pullBackError` says so; `skd update <pack>` finishes it.

## Changing skills

### `skd new <name> [--private | --in <pack> [--path <dir>]]`

Scaffolds a skill of your own and links it. The name follows the [skill name rule](../README.md#how-skills-are-found-in-a-repository).

- In `skills/` by default, published with the library by `skd sync`; in `private/` with `--private`, never pushed.
- `--in <pack>` makes it in [your own repository](#your-own-repository) instead, as a draft: in `vendor/` like the pack's other skills (linked, edited in place), and in the repository once `skd contribute <pack> --push` sends it. `skd list` marks it `+`. The pack's `use` gets its name unless the pack follows everything.
- Its place in the repository is `--path <dir>` (a directory of the repository, `.` for its top), else the directory most of the repository's skills are in (`skills/` when it has none). A hidden, test or dependency directory is refused (skd's scan would never find the skill there), and so is a repository that is one skill at its top unless `--path` says where.
- A name the library already uses is refused.
- `--json` prints `{created, path, pack, repoPath, draft, apply}` with `--in`.

### `skd own [pack] [--off]`

Marks a pack as your own repository (`own = true` in `skills.toml`), `--off` takes the mark away; without a pack it lists the packs marked. See [Your own repository](#your-own-repository).

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

## Your own repository

A GitHub repository of skills you keep yourself is a pack like any other, marked `own = true`:

```toml
[[pack]]
repo = "me/my-skills"
own  = true
use  = ["*"]
```

```bash
skd add me/my-skills --all --own        # or: skd own me/my-skills, for a pack you already follow
skd new notes --in me/my-skills         # a draft in vendor/, linked; edit it in place
skd contribute me/my-skills --dry-run   # what would be sent: changed skills, drafts and where they go, the diff
skd contribute me/my-skills --push --yes
skd update me/my-skills                 # what you pushed from elsewhere, merged into your copies
```

What the mark changes:

- Skills can be made in it (`skd new --in`), and `skd contribute <pack>` sends every change and new skill at once, pulling before and after a push.
- It may have no skill yet (`skd add --own`).
- No license warning (`add`, `update`, the dashboard): the skills are yours to redistribute.
- The wording: your edits are "not pushed" rather than "changed against upstream".

What it does not change: where the skills are kept (`vendor/`), how updates merge and shelve, links and targets, the pinned ref. Take the mark away (`skd own <pack> --off`) and the pack is an ordinary one again.

An older skd ignores `own`, and drops it when it saves `skills.toml`; it ignores a draft's `draft` and `path` in `skills.lock` too, and drops them when it saves it. Nothing is lost: a vendored skill with no hash is a draft all the same, its place worked out again when it is pushed, and `skd own <pack>` marks the pack again.

**Or make your repository the library.** If the repository is private and only for your own machines, it can be the library itself: your skills in `skills/`, pushed with `skd sync`. Everything the library holds is pushed with it, though: `skills.toml`, `skills.lock`, the history, the vendored copies of other people's skills in `vendor/` and the upstream snapshots. That is no place for a public repository of skills (it would redistribute everyone else's, and `npx skills add` would find them too), its skills must sit in `skills/`, and there is one library: so a public or shared repository is better kept as an own pack.

## Taking over and cleaning up

### `skd adopt [-n] [--yes]`

Takes over unmanaged entries found by `skd scan`: subscribes to their repository, registers apps, copies the rest in as your own, and enables each for the agents it was found in.

- A copy in the own directory of an agent that reads `~/.agents/skills`, of a skill the library already links into `agents`, is redundant: when it is the same as the library's, it goes to the trash (that agent keeps reading the library's copy through `agents`); when it differs, it is left in place with a note.
- A copy whose repository skd knows is compared with upstream first (one fetch per repository). If you changed it, it is copied in as your own skill rather than replaced by upstream's; the proposal names the repository and the `skd add` that would follow upstream instead (`Upstream` and `Differs` in `--json`).
- A copy whose content is exactly a library skill's, under whatever name, is taken for that skill: it is installed for the agent and the copy goes to the trash.
- A link into another skilldots library (one made before, at another path) is taken as that library has the skill, never as a clone of that library's repository: a vendored skill is a subscription to its own upstream at the ref the old pack follows, a skill of its `private/` is copied into `private/` (never pushed), one of its `skills/` into `skills/`, and the adopted skill is installed for exactly the agents the old library installs it for (`From`, `Ref`, `Private` and `Targets` in `--json`). Where the link keeps out a link of this library's (the `skilldots` skill a new library installs), this library's takes its place. The other library is only read: nothing in it changes, and its links go to the trash, so `skd trash restore` brings them back.
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
