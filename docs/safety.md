# What skd changes, and what it never does

skd is mostly run by agents, on libraries that may come from someone else (`skd clone`). This page lists the rules that keep that safe: what skd deletes, what it only moves to the trash, which directories it writes into, and what it prints. The [README](../README.md) has the short version.

## The trash

Nothing is deleted. `remove`, `purge` and `apply --force` move files into the library's trash, `<library>/.trash/`. `skd trash` lists it and `skd trash restore <id|skill|owner/repo>` puts an entry back.

Merging works the same way. Before an update (or `skd resolve`) writes a merge into a vendored skill you edited, the skill's directory as it was goes to the trash, with the reason `merged`; anything in it that is not one of the skill's files (a `.git`, `node_modules`, a link) stays in that copy. Restoring it swaps it back: the merged directory goes to the trash in its place (restorable the same way), and `skills.lock` goes back to what it recorded before the merge, so the next update merges again. Restoring the copy a skill being resolved started from is `skd resolve --abort`. A shelved update changes nothing in the skill at all. `skd resolve --abort` moves the half-resolved files to the trash too (reason `resolve-aborted`) before putting the copy back, and puts the library's git index back as it was. The upstream snapshots skd records (`refs/skilldots/upstream/*`) are written with a temporary index: the library's own index, work tree and branches are not touched.

`skd trash empty` is the one irreversible command, and it only runs for [a person at a terminal](#a-person-at-a-terminal). So an agent can do everything short of permanent deletion. The desktop app's Trash page can also delete for good, the whole trash or one entry, but only after a confirmation that names the entries and their size, and it refuses when the trash changed since that confirmation was shown.

### What skd removes outright

Agent directories only ever contain symlinks into the library (copies on Windows without symlink rights, or for targets with `mode = "copy"`), so a skill exists once on disk. skd removes outright only what it can tell it made:

- a link that points into the library, at the place the lock records;
- a link exactly as skd's own record says it placed it;
- a copy only when that record says skd wrote exactly it;
- a vendored directory only when it matches the lock.

The record is `placed.json`, beside the settings file and never part of a library. A link or copy made before skd kept the record is recorded by the next `skd apply`, while it is exactly what skd would make: a link to the library, or a copy identical to its source with nothing added.

Everything else goes to the trash, or stays:

- Whatever `skills.lock` says, anything else in the way of a wanted skill goes to the trash, your own directories and links included.
- An entry the manifest no longer wants that skd cannot tell it made is left in place, with `--force` too.
- A link skd moves aside (one of yours, with `--force`, or a broken one that `adopt` clears) is listed in the trash too, but you move it back by hand.

### Library files that are links

`skills.toml`, `skills.lock` or the update history may be a link. skd reads through it, but writes through it only when it leads to a plain file directly in the library's folder: never outside the library, into `.git/`, or to a hidden file.

### What `skd trash restore` puts back

It only puts a directory back where skd takes them from:

- an entry of an agent's skill directory (for an approved custom target, only an entry named like a skill, so never `.ssh`);
- a place inside the library's `vendor/`;
- a skill directory of the library's `skills/` or `private/` that `skd remove` moved there on this machine. skd records that beside the settings file, in `local-trash.json`, never in the library, so a cloned library's trash cannot put files into your `skills/`.

What `remove` and `purge` took out of `skills.toml` and `skills.lock` is recorded the same way, in `trash-notes.json` beside the settings file (written under `trash-notes.lock`, so a desktop app and an agent's `skd` running at once lose nothing; a damaged file is kept as `trash-notes.json.broken-<time>` and a fresh one started; deleting an entry for good forgets its record), so restoring their entry also puts the manifest and lock back and relinks. An entry from before that record, or from another machine, gets its files back and a hint naming the `skd add` that follows it again.

A trash entry recorded as coming from anywhere else, or reached through a symbolic link, is listed as not restorable, with the reason, and left for you to move by hand. A library cloned from someone else carries their `.trash/` too, so such a record, and where a link entry leads, is never shown: it may be anyone's text.

An entry not named the way skd names what it sets aside is listed as `(unsafe name)`, numbered from the second on so each ID is different (`"unsafe": true` in `--json`), and skd does not restore it. What skd sets aside itself is always named so (for example the vendored `.github` of a pack `owner/.github` goes in as `github`) and goes back under its own name.

`skd trash restore` also does not restore to a recorded origin that holds control characters ([below](#what-skd-prints)).

### Links made by an older skd

Links an older skd made, before it kept its record, are treated on the first command after upgrading the way that skd treated them:

- An app's links are recorded the first time skd opens the library your settings name (the one `skd init`, `skd use` or `skd clone` recorded; when they name none, not even `~/.skilldots`, nothing is), before that command changes anything, while each is exactly a link to the app's own directory that `skills.toml` names and `skills.lock` records. So `skd disable <app>` (or `--agent`) as the very first command still removes them outright.
- If you take the `[[app]]` out of `skills.toml` by hand before running any skd command, they are left in place, like any link skd cannot tell it made, and you remove them yourself.
- That first command, even a read-only one such as `skd list`, writes `placed.json` beside the settings file (empty when there is nothing to record). Its being there is what says it was done.
- If it cannot be written (a read-only settings folder), the links are taken for skd's in that run only, skd says so once on stderr, and the next command looks again.
- This happens only once per machine, and never for a library skd is pointed at by `skd clone` or `skd use`, `--library`, `$SKILLDOTS_LIBRARY` or the current directory. So a cloned library's `skills.toml` and `skills.lock` cannot make a link of yours look like skd's.

### Moved or copied libraries

After `skd use <new dir>` and `skd apply`, links into the old location are recognised and re-pointed:

- One skd recorded making, or one that leads nowhere now, is removed outright. Any other goes to the trash or, for a skill no longer wanted, stays where it is.
- After the library is moved (`mv`), a link to its old location is removed when it leads nowhere because that location is gone from a folder that is still there (not when a volume is unmounted).
- After it is copied (`cp -R lib lib2`, then `skd use lib2`), a link into the old copy is relinked without going to the trash when the old copy's own `skills.lock` records that very link.

## What is pushed, where, and when

skd never pushes, forks or opens a pull request on its own: not on update, not on a schedule, not in the background.

- `skd sync` (and `skd update --commit`) pushes the library to its own git remote, with the upstream snapshots (`refs/skilldots/*`). It first merges what that remote has; a conflict stops it with the library unchanged. `private/` is never pushed.
- `skd contribute` is the only thing that sends a change to a pack's upstream repository, and only the one skill named: to the pack's branch (`--push`) or as a pull request (`--pr`, a branch on the repository or on your fork). Without a terminal it needs `--yes`; the desktop app shows the repository, the branch, the mode and the diff, and sends on a confirmation. It commits in a clone of its own in skd's cache, with your git identity, never in the library.
- What gh is asked (whether you are logged in, whether you may push, a fork, a pull request and its state) happens only during `skd contribute` (and, for the state of pull requests already opened, `skd contribute --refresh` and `skd outdated`).

## A person at a terminal

`skd trash empty` and approving a custom target are only for a person at a terminal. That means stdin and stdout are terminals and, where skd asks, the answer was typed.

This keeps out agents' ordinary tool calls, scripts, cron and `--yes`. It does not keep out a program that drives a pseudo-terminal itself (`script`, `expect`, a terminal it controls), which can answer like a person. skd cannot tell the difference; the bundled `skilldots` skill forbids agents to do it.

## Custom targets

A `[targets.custom.<name>]` in `skills.toml` names a directory for skd to write into. Because a `skills.toml` may come from someone else's library, and skd writes links and copies into a target, restores trash entries there and, with `--force`, moves what is in the way into the trash, a custom target needs your approval on each machine.

### Approving

- `skd targets trust <name>` shows the directory the target leads to (`~` expanded, symbolic links resolved) and its mode, and asks. `T` in the dashboard does the same: it lists the targets, and `a` approves one.
- A directory not named `skills` (or `skill`, in any case) comes with a warning that whatever reads or runs files from it would load the skills. It is approved only by typing the target's name, not by answering yes.
- The approval is the name together with that directory and that mode (`link` or `copy`). It is kept beside the settings file (`trusted-targets.toml`), never in a library. When `skills.toml` points the target somewhere else or changes its mode, it needs approving again. An approval recorded by an older skd, without a mode, counts for `link`.
- `skd targets untrust <name>` withdraws the approval.
- Without a terminal, with `--yes` or with `--json`, nothing can be approved, so an agent cannot do it through its ordinary commands. (One that drives a pseudo-terminal could; see [above](#a-person-at-a-terminal).)
- `skd clone` lists the library's custom targets before it applies and, at a terminal, asks about each.

Until it is approved, the target is ignored everywhere (apply, enable, add, update, restore, the dashboard), with a warning saying why.

A custom target may not take a built-in target's name: the built-in one is used and the custom one ignored.

### Directories refused even with approval

- anything outside your home directory, and the home directory itself;
- a path through a symbolic link that leads nowhere (it would lead wherever something appears later);
- anything that is, is inside or holds a place whose files hold keys or are loaded or run by themselves:
  - keys and credentials: `~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.azure`, `~/.kube`, `~/.docker`, `~/.password-store`, `~/.config/gcloud`, `~/.config/gh`, `~/.config/git`, `~/Library/Keychains`;
  - started by the system: `~/.config/autostart`, `~/.config/systemd`, `~/.config/environment.d`, `~/.local/share/systemd`, `~/Library/LaunchAgents`, `~/Library/LaunchDaemons`, the Windows Startup folder;
  - programs: `~/.local/bin`, `~/bin`, `~/go/bin`, `~/.cargo`, `~/node_modules`, `~/Applications`, `~/.local/share/applications`;
  - shells: `~/.oh-my-zsh`, `~/.zsh`, `~/.config/zsh`, `~/.config/fish`, `~/.bashrc.d`, `~/Documents/PowerShell`, `~/Documents/WindowsPowerShell`;
  - editors: `~/.vim`, `~/.config/nvim`, `~/.local/share/nvim`, `~/.emacs.d`, `~/.config/emacs`, `~/.vscode`, `~/.vscode-insiders`, `~/.vscode-server`, `~/.cursor/extensions`, `~/.config/Code`, `~/Library/Application Support/Code`;
  - other things that load files: `~/.git-templates`, `~/.hammerspoon`, `~/Library/Services`, `~/Library/Scripts`, `~/Library/Workflows`, `~/Library/PreferencePanes`, `~/Library/QuickLook`, `~/Library/Input Methods`;
- skd's own settings directory, and the library itself;
- anything that holds a built-in target's directory (such as `~/.claude`) or is inside one (such as `~/.claude/skills/foo/x`): skd writes that directory's entries itself, and a skill linked there may hold a link that leads anywhere;
- anything that is, is inside or holds the directory of another custom target you approved. (An unapproved one is not used anyway, so a library cannot hold back a target you approved by adding one inside it.)

### Writing into a target

Every write into a target, built-in or custom, first checks that its path still leads to the directory found when the library was opened (for a custom target, the one approved). When a link on the way appeared or changed since, even one skd itself made earlier in the same run, nothing is written, moved aside or removed there, and apply says so.

### Targets no longer in use

- Links skd made in a custom target that is then withdrawn or removed from `skills.toml` stay where they are. Every `skd apply` names them and `skd scan` lists them as `left`, without suggesting to remove them (the record comes with the library, and the link it names may be your own), until they are gone.
- A `skills.lock` record of a target not in use (removed from `skills.toml`, not approved, refused, or never defined) is only forgotten, never acted on, with `--force` too. A link it made stays where it is; apply names the path only when it is a link into the library, never as something to remove.
- To have skd remove such links, disable the skill for that target while the target is still in use: `skd disable <skill> --agent <name>`.

## Application skills (`[[app]]`)

An `[[app]]` is linked (or copied) only when its `path` is a skill: a directory holding `SKILL.md` as a plain file, named by an absolute path or one starting with `~/`. (A relative path would be checked from wherever skd runs but read by the agent from its own directory.)

Skipped with a warning, and not forked either:

- a relative path;
- a path without `SKILL.md`, or one whose `SKILL.md` is a link;
- a directory that is, is inside or holds a place of keys and credentials (`~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.config/gh`, `~/Library/Keychains`, … or skd's own settings).

Otherwise a `skills.toml` from someone else's library could have skd link `~/.ssh` into every agent, or copy it into a copy target.

## Symbolic links inside the library

- A `LICENSE` that is a symlink is never copied.
- Nor is anything reached through a symbolic link inside the library. When `skills/`, `private/`, `vendor/`, a pack's or a category's directory, or a skill's own directory is a link, the skills under it are neither linked nor copied into any agent, and `skd update` of that pack stops before it writes, moves or removes anything there. skd says which link it is; make it a directory of its own.
- The library itself may be reached through a link.

## What skd prints

Nothing skd prints for a person carries control characters (escape sequences, BEL, C1) or bidi controls from an upstream repository, a library file, an agent directory or the settings file. So a hostile `SKILL.md`, or a hostile library someone hands you, cannot rewrite your terminal.

Names and paths are refused, not cleaned:

- A `skills.toml` or `skills.lock` with such characters anywhere is refused, with each place named and quoted, and skd never writes one. "Anywhere" means any value, list element or key: repo, ref, alias, `use`, `skip`, `targets`, `[pack.skill.<name>]` and its `as`, apps, locals, custom targets and their `mode`, every lock record.
- A table or key skd does not read (your own notes, a newer version's field) may hold tabs and line breaks, but not an escape, another control character, a bidi control or invalid UTF-8.
- `skd trash restore` does not restore to a recorded origin that has them.
- `skd adopt` skips an entry whose name is not a valid skill name.
- `--alias`, `config set`, `sync --remote` and `clone` refuse such values.

Text that is only shown is cleaned as it is read or printed: a `description` (in `--json` too), upstream's commit subjects and the rest of the history, trash entries, agent directory entries, settings, warnings and errors, git's output. A skill whose category directory holds such characters is skipped with a warning, like an unsafe name.

Behind all that, whatever skd prints for a person goes through a last filter that lets only line breaks and colour codes through, and only colour codes that cannot hide text (no conceal or blink, no text the colour of its background). The dashboard does the same with every screen.

A test runs every command, and the dashboard's views, against a library that is hostile in all these places.

Under `--json` the other fields are data as found: every control character in them (DEL, C1 and bidi controls included) is written as a `\u` escape, so the document decodes to the same values and is harmless on a terminal. A program that shows them to a person cleans them itself.

## Git

skd runs `git` for everything it fetches and commits; 2.24 or later is best. An older git does not know `--end-of-options`, which skd puts before arguments read from files (commits from `skills.lock`). skd then passes them without it, but only when none of them could be taken for an option, and otherwise fails saying it needs git 2.24.
