# skilldots

Declarative skill management for AI coding agents. One git-backed library, linked into every agent you use.

```
skd add mattpocock/skills          # pick skills from a repo; vendored into your library
skd add trailofbits/skills -s differential-review
skd add "npx skills add vercel-labs/agent-skills --skill frontend-design"   # pasted install line
skd edit tdd                       # open a skill in your editor
skd list                           # one line per repo, not one per skill
skd update                         # show what changed upstream, update the packs you tick
skd history                        # what each update changed, with upstream's commit messages
skd disable grill-me --agent codex # per-skill, per-agent control
skd purge mattpocock/skills        # gone from the library and every agent dir
```

## Why

Skills are just folders with a `SKILL.md`, and every agent reads them from its own directory (`~/.claude/skills`, `~/.codex/skills`, `~/.agents/skills`, …). Installers copy files into those directories and then forget about them: you lose track of what came from where, updates overwrite silently, and removing a skill means hunting through a dozen folders.

skilldots treats it like dotfiles:

- **`skills.toml` is the source of truth.** It says which repositories you follow, which skills you picked, and which agents get them. `skd apply` makes the machine match it.
- **The library is a git repository.** Third-party skills are vendored in, so an upstream change shows up as a diff you can read and revert. A new machine is `git clone` + `skd apply`.
- **Repositories are the unit, skills are the selection.** A repo with 40 skills is one line in `skd list` and one `git fetch` in `skd update`, however many of its skills you use.
- **Your own skills live alongside.** `skills/` is for ones you publish with the library; `private/` is a separate local git repo that is never pushed.
- **Every link is recorded.** `skd` only ever touches files it placed itself, and tells you when something else is in the way.

## Built for agents

Most of the time it is not a person running `skd` but the agent on their machine — Claude Code, Codex, Cursor and friends. skilldots is designed around that:

- `skd init` installs the [`skilldots` skill](skills/skilldots/SKILL.md) into the library, so every agent on the machine knows how to add, update, fork and remove skills through `skd` instead of poking at directories.
- Every command works without a terminal (stdin or stdout not a terminal, so `skd update | cat` never waits on a hidden prompt): `--json` for structured results, `--yes` for prompts, `skd show owner/repo --json` to inspect a repository before `skd add --skill …`.
- Nothing is deleted. `remove`, `purge` and `apply --force` move files into the library's trash; `skd trash` lists and restores them. skd removes outright only what it can tell it made: a link into the library, or a link or copy exactly as its own record says it placed it (`placed.json` beside the settings file, never part of a library). Whatever `skills.lock` says, anything else in the way, your own directories and links included, goes to the trash (and `skills.toml`, `skills.lock` or the update history that is a link is read but written through only when it leads to a plain file directly in the library's folder, never outside the library, into `.git/` or to a hidden file), and an entry the manifest no longer wants that skd cannot tell it made is left in place, with `--force` too. `skd trash empty` is the one irreversible command and it only runs for a person at a terminal, so an agent can do everything short of permanent deletion. "A person at a terminal" means stdin and stdout are terminals (and, where it asks, the answer typed): it keeps out agents' ordinary tool calls, scripts, cron and `--yes`, not a program that drives a pseudo-terminal itself (`script`, `expect`, a terminal it controls), which can answer like a person. The bundled skill forbids agents to do that; skd cannot tell the difference. Approving a custom target (a directory `skills.toml` names for skd to write into) is likewise for a person at a terminal: see [skills.toml](#skillstoml).
- People get a full-screen dashboard (`skd` with no arguments) that does everything the commands do, for looking around and vetoing.

## Install

```sh
go install github.com/Maybe404/skilldots/cmd/skd@latest
```

Prebuilt binaries will follow. skd runs `git` for everything it fetches and commits; 2.24 or later is best. An older git does not know `--end-of-options`, which skd puts before arguments read from files (commits from `skills.lock`): skd then passes them without it, but only when none of them could be taken for an option, and otherwise fails saying it needs git 2.24.

## Library layout

```
~/.skilldots/                     # or any directory: skd init <dir>, or $SKILLDOTS_LIBRARY
├── skills.toml                   # what you want
├── skills.lock                   # what is vendored at which commit, and what is linked where
├── skills.history.jsonl          # what each update changed (skd history); committed with the library
├── skills/<name>/                # skills you write; pushed with the library
├── private/<name>/               # skills for this machine only; its own git repo, git-ignored here
└── vendor/<owner>/<repo>/[category/]<name>/   # third-party, written by skd
```

## Commands

| | |
|---|---|
| `skd init [dir]` | create a library (its own git repository, even inside another repo; `private/` is a separate one); the default target is `agents` (`~/.agents/skills`), which nearly every agent reads |
| `skd show <owner/repo \| url>` | list what a repository offers, without installing |
| `skd add <owner/repo \| url \| "pasted install line">[@ref]` | fetch a repo and choose skills interactively, or with `--skill`, `--category`, `--all` (`--all` follows everything; names given with it are only checked). On a pack that follows `"*"`, `--skill x` takes `x` out of `skip`; if a wildcard such as `eng/*` excludes it, the command fails and tells you to edit `skills.toml`. Also takes a whole `npx skills add owner/repo --skill x` line (bunx, pnpx, `pnpm dlx`, `yarn dlx`, `npm exec`, `npx add-skill`, `skd add`; quote it), a skills.sh skill page, or a GitHub link to a skill directory (`/tree/<ref>/<dir>`) or its `SKILL.md` (`/blob/<ref>/…`): the repository is taken from it and the skills it names are ticked in the picker (without a terminal they are the selection). The text is only parsed, never run; unquoted, only an install command may span several arguments (a local path with a space needs quotes too), and a source or `@ref` starting with `-` is refused. The `<ref>` in a GitHub link is where the page was browsed and is not pinned; append `@ref` to pin. With no source at a terminal it asks for one. |
| `skd list [pack] [--all] [--flat] [--agent x]` | overview; expand a pack; flat list; what one agent receives |
| `skd apply [-n] [--force]` | link/unlink to match the manifest; `-n` previews. `--force` replaces entries in the way of a wanted skill (moved to the trash); an entry no longer wanted is removed only when skd can tell it made it, otherwise it stays and the plan says so |
| `skd update [pack…] [--commit]` | at a terminal: check upstream, list each pack's changes (changed / new / gone / moved), every pack ticked; only the ticked packs are updated (ctrl+c cancels). Named packs with nothing new upstream are offered an "update anyway", which re-vendors missing skills and re-links (as `u` in the dashboard); with none named it points to `skd apply`. With `--yes`, `--json` or no terminal it updates straight away, every pack when none are named. Then re-vendors the selected skills, reports, and records the update |
| `skd history [pack] [-n N]` | what each update changed: when, old → new commit, the skills changed / added / moved / gone / new upstream, and the subjects of up to 20 upstream commits that touched them. An update whose linking failed says so until an apply links every skill of that pack (one still skipped, say because something is in the way, keeps the note). Read from `skills.history.jsonl` in the library (committed and pushed with it), so it works offline and on another machine after a clone |
| `skd outdated [pack…]` | check upstream without changing anything: per pack, the selected skills that changed (a skill only moved to another directory is listed as `moved` and is not an update: a pack with nothing but moves is not counted and `U` in the dashboard skips it, though `u` or `skd update` on it still renames the vendored directory to the new path), skills new upstream since the last update (or since `add`, for a pack never updated), skills gone upstream; exits non-zero if a pack could not be fetched. `skd list` and the menu show the last result (`↑ 2 updates available · checked 2h ago`, stale after a day) without fetching |
| `skd enable <skill\|pack> [--agent a,b]` | link into agents (default set when omitted) |
| `skd disable <skill\|pack> [--agent a,b]` | unlink from agents (all when omitted); the library keeps it |
| `skd remove <skill>` | deselect a vendored skill: unlinked everywhere, dropped from `vendor/` |
| `skd purge <pack>` | delete a pack from manifest, lock, `vendor/` and every agent directory |
| `skd fork <skill> [--private] [--as name]` | copy a vendored skill into `skills/` to edit it; upstream no longer touches it |
| `skd path <skill>` | print where a skill's files live, for editing |
| `skd edit <skill> [--fork [--private] [--as name] \| --force]` | open a skill's directory in your editor: the `editor` setting, `$VISUAL`, `$EDITOR`, then VS Code, Cursor, Zed or Sublime Text (command or macOS app), then the system opener. A vendored or app skill is replaced on update, so at a terminal skd offers to fork it first; without a terminal it refuses unless `--fork` (fork, open the copy) or `--force` (open the files anyway) is given. A terminal editor (vim, nano) needs a terminal. A command skd does not know as a window editor (code, cursor, zed, subl and the like) is taken for a terminal editor; if yours opens a window, `skd config set editor-kind window` says so (and `terminal` the other way). When the editor cannot run (not installed, or a terminal editor without a terminal) `--fork` fails before forking anything |
| `skd config list \| get <key> \| set <key> [value]` | settings, per user and machine: `editor` (command), `editor-kind` (`auto`, `window`, `terminal`: whether that command opens a window or needs the terminal), `theme` (`light`, `dark`, `auto`), `lang` (reserved; only English so far); `library` is shown and set by `skd use`. No value unsets. `set` rewrites only that setting's line, so comments in the file stay, a comment at the end of that line too (unsetting removes the line with its comment; a value spanning several lines makes skd write the file again without comments), and new lines end like the file's (LF or CRLF). A settings file that is not valid TOML is reported once, with its path and the parse error: as the error of a command that needs it (finding the remembered library, `config`, `use`, and `init` and `clone`, which check before creating anything), or as one warning after a command that ran without its settings |
| `skd new <name> [--private]` | scaffold a skill of your own and link it; the name follows the skill name rule below |
| `skd scan` | inventory every agent skill directory: managed, copies, links, broken; and links skd made in a custom target no longer in use (`left`), which it leaves for you to remove |
| `skd adopt [-n] [--yes]` | take over unmanaged entries: subscribe, register apps, copy as your own, enable per agent |
| `skd trash [restore <id> \| empty]` | what skd set aside; restore it; or (interactively) delete for good |
| `skd targets [trust <name> \| untrust <name>]` | known agent directories, which exist on this machine, and the custom targets of `skills.toml` that are not used, with why; `trust` approves a custom target (a person at a terminal only), `untrust` withdraws that |
| `skd clone <git-url> [dir]` | set up a machine from a library you pushed elsewhere; lists its custom targets first and, at a terminal, asks whether to approve each. A `private` that is a symbolic link in the cloned library is left as it is (no git repository is set up through it, and the skills under it are skipped); `skd init --from` refuses one |
| `skd sync [--remote url] [-m msg]` | commit the library and push it: your backup |
| `skd use <dir>` | point skd at an existing library from any directory |
| `skd [--simple]` | with no arguments: the full-screen dashboard (below); `--simple`, a narrow terminal or `TERM=dumb` give a question-and-answer menu instead |

All commands take `--json` for scripts and agents, `--yes` to skip prompts, and `--theme light|dark|auto` for colours (default `auto`, also `$SKILLDOTS_THEME`, then `skd config set theme`, where a value skd does not know falls back to `auto` with a warning; `NO_COLOR` turns colour off). Under `--json` stdout carries exactly one JSON document (progress and warnings go to stderr); a failing command prints nothing on stdout and exits non-zero, except `skd update`, which reports `{packs, failed, apply, committed, history}` and exits non-zero when any pack failed (and, with an `error` field, when the update was saved but linking or `--commit` then failed), and `skd outdated`, which does the same with `{checked, packs, failed, updates}` (each pack: `repo, commit, latest, changed, moved, new, gone, pending, checked`), and `skd apply`, which prints its document and exits non-zero when a planned change was not made because its agent directory no longer led where it did, or its source went through a symbolic link inside the library (`refused`; the last line of the plain output then reads `applied · … · N not changed` instead of a tick). Problems confined to one skill — an unknown target, a name clash, a pack not fetched yet — are warnings, not errors: the rest of the library keeps working.

Nothing skd prints for a person carries control characters (escape sequences, BEL, C1) or bidi controls from an upstream repository, a library file, an agent directory or the settings file, so a hostile SKILL.md or a hostile library someone hands you cannot rewrite your terminal. Names and paths are refused, not cleaned: a `skills.toml` or `skills.lock` with such characters anywhere (any value, list element or key: repo, ref, alias, `use`, `skip`, `targets`, `[pack.skill.<name>]` and its `as`, apps, locals, custom targets and their `mode`, every lock record) is refused with each place named and quoted, and skd never writes one. A table or key skd does not read (your own notes, a newer version's field) may hold tabs and line breaks, but not an escape, another control character, a bidi control or invalid UTF-8; `skd trash restore` does not restore to a recorded origin that has them, `skd adopt` skips an entry whose name is not a valid skill name, and `--alias`, `config set`, `sync --remote` and `clone` refuse such values. Text that is only shown is cleaned as it is read or printed: a `description` (in `--json` too), upstream's commit subjects and the rest of the history, trash entries, agent directory entries, settings, warnings and errors, git's output; a skill whose category directory holds such characters is skipped with a warning, like an unsafe name. Behind all that, whatever skd prints for a person goes through a last filter that lets only line breaks and colour codes through, and only colour codes that cannot hide text (no conceal or blink, no text the colour of its background), and the dashboard does the same with every screen. A test runs every command (and the dashboard's views) against a library that is hostile in all these places. Under `--json` the other fields are data as found (every control character in them, DEL, C1 and bidi controls included, is written as a `\u` escape, so the document decodes to the same values and is harmless on a terminal); a program that shows them to a person cleans them itself.

## Dashboard

`skd` in a terminal at least 90 columns wide opens a dashboard: packs and your own skills on the left, the selected pack's skills on the right. It runs the same code as the commands, so anything you do there leaves the library exactly as the matching command would.

| key | does | command |
|---|---|---|
| `↑↓` `j k` `pgup pgdn` `g G` | move | |
| `tab` `← →` `l` `enter` | switch column, open a pack | |
| `/` | filter the column (`esc` clears) | |
| `space` | enable / disable the skill; installs an upstream one | `enable`, `disable`, `add --skill` |
| `a` | add a repository (or paste an install line or a skills.sh / GitHub link: what it names is ticked): tick skills or whole categories (or `*` for everything), then an optional alias and agents | `add [--category] [--alias] [--agent]` |
| `u` / `U` | update this pack / every pack with upstream changes: both check upstream first and show what would change, skill by skill; `u` asks to confirm, `U` lists the packs ticked and updates the ones you leave ticked | `update` |
| `h` | update history: each update, and upstream's commit messages for it | `history` |
| `c` | check upstream again; the dashboard checks once in the background when it opens: `↑ N packs have updates` in the header, `↑` on the packs, `↑ changed`, `new upstream`, `gone upstream` (and `moved upstream`, not an update) on the skills | `outdated` |
| `e` | agents for the skill or pack | `enable/disable --agent` |
| `f` | fork, under a new name if you type one, into `skills/` or `private/` | `fork [--as] [--private]` |
| `o` | open the skill in your editor; a vendored or app one asks: fork and open the copy, or open anyway | `edit [--fork] [--force]` |
| `d` | remove a vendored skill, unlink one of yours, purge a pack | `remove`, `disable`, `purge` |
| `n` | new skill in `skills/` or `private/`, linked and selected | `new [--private]` |
| `S` | scan agent directories by agent and state; adopt what you tick | `scan`, `adopt` |
| `t` | trash: restore an entry, or empty it (asks twice; the second answer is `D`) | `trash restore`, `trash empty` |
| `T` | agent targets; the custom ones not in use come first, with why; `a` approves one after showing its directory | `targets`, `targets trust` |
| `s` | back up: commit and push | `sync` |
| `A` / `r` | apply pending link changes / reload | `apply` |
| `,` | settings: editor, editor kind, theme (applies next start) | `config` |
| `?` | every key | |
| `q` `esc` | quit; `ctrl+c` while something runs quits when it ends, twice stops it | |

While an operation runs, keys that would start another are refused; problems show on the status line.

**Skills Claude Code turned off.** Claude Code keeps skills a person turned off inside it in `skillOverrides` of `~/.claude/settings.json` (`$CLAUDE_CONFIG_DIR/settings.json` when set). skd reads that file — never writes it, and ignores it when missing or malformed — and marks those skills `off in Claude Code` — matched by link name, the name Claude sees, and only for skills linked into Claude's directory — in `skd list` (`ClaudeDisabled` in `--json`) and the dashboard. skd still links them; to keep one out of Claude entirely, remove the claude target (`skd disable <skill> --agent claude`, or `e` in the dashboard).

Colours follow the terminal: skd asks it for its background colour (giving up after 300 ms and assuming dark; it does not ask when you have already typed ahead, and keys typed while it waits are passed on to the dashboard) and uses a light or dark palette for the dashboard, the menu and every table. `--theme light` or `--theme dark` (or `SKILLDOTS_THEME`) skips the question.

## skills.toml

```toml
[targets]
default = ["agents"]                         # add agents that do not read ~/.agents/skills

[targets.custom.myagent]
path = "~/.myagent/skills"

[[pack]]
repo = "mattpocock/skills"
alias = "matt"                               # no "/", "\" or ":"; never a subscribed repo
use = ["tdd", "grill-me", "code-review"]     # or "*" to follow everything, with skip = [...]

[pack.skill.code-review]
as = "matt-code-review"                      # link name when another skill already uses the name
targets = ["claude"]                         # per-skill override

[[app]]                                      # a skill shipped inside an installed application
name = "surge"
path = "/Applications/Surge.app/Contents/Resources/Skills/surge"

[[local]]                                    # only needed to change targets of a skills/ or private/ entry
name = "idea-config-maintainer"
targets = ["agents"]
```

Targets inherit: global default → pack → skill. `targets = []` keeps a skill in the library without linking it anywhere.

An `[[app]]` is linked (or copied) only when its `path` is a skill: a directory holding `SKILL.md` as a plain file. A path without one, one whose `SKILL.md` is a link, or a directory that is, is inside or holds a place of keys and credentials (`~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.config/gh`, `~/Library/Keychains`, … or skd's own settings) is skipped with a warning, and is not forked either: a `skills.toml` from someone else's library could otherwise have skd link `~/.ssh` into every agent, or copy it into a copy target.

**Custom targets need your approval on each machine.** A `skills.toml` may come from someone else's library (`skd clone`), and skd writes links and copies into a target, restores trash entries there, and with `--force` moves what is in the way into the trash. So a `[targets.custom.<name>]` is used only after a person approves it at a terminal: `skd targets trust <name>` shows the directory it leads to (`~` expanded, symbolic links resolved) and its mode, and asks; `T` in the dashboard does the same (it lists the targets; `a` approves). A directory not named `skills` (or `skill`, in any case) comes with a warning that whatever reads or runs files from it would load the skills, and is approved only by typing the target's name, not by answering yes. The approval is the name together with that directory and that mode (`link` or `copy`), kept beside the settings file (`trusted-targets.toml`), never in a library: when `skills.toml` points the target somewhere else or changes its mode it needs approving again (an approval recorded by an older skd, without a mode, counts for `link`), and `skd targets untrust <name>` withdraws it. Links skd made in a custom target that is then withdrawn or removed from `skills.toml` stay where they are; every `skd apply` names them and `skd scan` lists them as `left` until you remove them (or disable the skill for that target while it is still in use). Without a terminal, with `--yes` or with `--json` nothing can be approved, so an agent cannot do it through its ordinary commands (one that drives a pseudo-terminal could; see [Built for agents](#built-for-agents)). Until then the target is ignored everywhere (apply, enable, add, update, restore, the dashboard), with a warning saying why; `skd clone` lists the library's custom targets before it applies and, at a terminal, asks about each. Some directories are refused even with approval: anything outside your home directory, the home directory itself, a path through a symbolic link that leads nowhere (it would lead wherever something appears later); anything that is, is inside or holds a place whose files hold keys or are loaded or run by themselves (`~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.azure`, `~/.kube`, `~/.docker`, `~/.password-store`, `~/.config/gcloud`, `~/.config/gh`, `~/.config/git`, `~/.config/autostart`, `~/.config/systemd`, `~/.config/environment.d`, `~/.local/share/systemd`, `~/Library/LaunchAgents`, `~/Library/LaunchDaemons`, `~/Library/Keychains`, the Windows Startup folder, `~/.local/bin`, `~/bin`, `~/go/bin`, `~/.cargo`, `~/node_modules`, `~/Applications`, `~/.local/share/applications`, `~/.oh-my-zsh`, `~/.zsh`, `~/.config/zsh`, `~/.config/fish`, `~/.bashrc.d`, `~/.vim`, `~/.config/nvim`, `~/.local/share/nvim`, `~/.emacs.d`, `~/.config/emacs`, `~/.vscode`, `~/.vscode-insiders`, `~/.vscode-server`, `~/.cursor/extensions`, `~/.config/Code`, `~/Library/Application Support/Code`, `~/.git-templates`, `~/.hammerspoon`, `~/Library/Services`, `~/Library/Scripts`, `~/Library/Workflows`, `~/Library/PreferencePanes`, `~/Library/QuickLook`, `~/Library/Input Methods`, `~/Documents/PowerShell`, `~/Documents/WindowsPowerShell`), skd's own settings directory, or the library itself; anything that holds a built-in target's directory (such as `~/.claude`) or is inside one (such as `~/.claude/skills/foo/x`: skd writes that directory's entries itself, and a skill linked there may hold a link that leads anywhere); and anything that is, is inside or holds the directory of another custom target you approved (an unapproved one is not used anyway, so a library cannot hold back a target you approved by adding one inside it). A custom target may not take a built-in target's name: the built-in one is used and the custom one ignored. Every write into a target (built-in or custom) first checks that its path still leads to the directory found when the library was opened (for a custom target, the one approved); when a link on the way appeared or changed since, even one skd itself made earlier in the same run, nothing is written, moved aside or removed there, and apply says so. A `skills.lock` record of a target not in use (removed from `skills.toml`, not approved, refused, or never defined) is only forgotten, never acted on, with `--force` too: a link it made stays where it is, and apply prints the path for you to remove by hand (to have skd remove them, `skd disable <skill> --agent <name>` while the target is still in use).

## How skills are found in a repository

Every directory containing a `SKILL.md` is a skill. Test directories are skipped, and so are hidden directories (except `.agents/`, the cross-agent convention) — unless the repository has skills nowhere else, as when it ships them only in `.claude/skills/`. Repositories that copy the same skill into many agent folders are collapsed to one, preferring `skills/` or `.agents/skills/`; `skd show` lists the other copies whose content differs.

The category is the skill's parent path with structural segments (`skills`, `skill`, `plugins`, `plugin`, hidden directories) removed: `skills/engineering/tdd` is `engineering`, `plugins/code-improver/skills/pr-improver` is `code-improver`. It depends only on that skill's own location, so new directories upstream never recategorise existing skills; when a skill does move, its vendored directory is renamed.

A skill's name (frontmatter `name`, else its directory) must be letters, digits, `.`, `_` and `-`, not starting with `.`. Skills with any other name are skipped with a warning, because the name becomes a directory and link name. The same rule applies to `as`, `[[app]].name` and `[[local]].name`. Within one repository, a skill whose name differs only in case from another, or whose category would put it inside another skill's directory, is skipped with a warning. A `LICENSE` that is a symlink is never copied. Nor is anything reached through a symbolic link inside the library: when `skills/`, `private/`, `vendor/`, a pack's or a category's directory, or a skill's own directory is a link, the skills under it are neither linked nor copied into any agent, `skd update` of that pack stops before it writes, moves or removes anything there, and skd says which link it is (make it a directory of its own). The library itself may be reached through a link.

## Name clashes

The first skill to use a name keeps it. When `skd add` or `skd update` vendors a skill whose name is already taken, it is linked as `<repo>-<name>` and the `as` is written to `skills.toml`. Names are compared case-insensitively. Two skills may share a name if they go to different agents (commands then need a qualified name: `owner/repo:foo` or `<alias>:foo`, `skills:foo`, `private:foo`, `app:foo`, and fail listing the candidates otherwise); if they would land in the same agent directory (for example you create `skills/foo` while a pack provides `foo` to the same agents), both are skipped with a warning naming both directories, existing links stay as they are, and everything else is linked as usual. `skd fork` renames the frontmatter to the fork's name, so a fork never clashes with its upstream.

## Editing, rolling back, removing

- **Third-party skill you want to change:** `skd fork <skill>`. The copy in `skills/` is yours; the vendored original is deselected so the names do not clash.
- **Your own skill:** `skd edit <skill>` (or `o` in the dashboard) opens it; edit in place and commit.
- **What did an update change?** `skd history [pack]`: per update, the skills that changed and upstream's commit messages for them.
- **Roll back an update:** the library is a git repository. With `skd update --commit` every update is one commit, so `git revert <commit> && skd apply` undoes it. Without `--commit`, `git checkout -- vendor skills.lock && skd apply`.
- **Remove:** `skd disable` unlinks but keeps the files; `skd remove` also drops a vendored skill from the library; `skd purge` deletes a whole pack.

- **Moved the library:** `skd use <new dir>` then `skd apply`; links into the old location are recognised and re-pointed. One skd recorded making, or one that leads nowhere now, is removed outright; any other goes to the trash, or, for a skill no longer wanted, stays where it is.

Agent directories only ever contain symlinks into the library (copies on Windows without symlink rights, or for targets with `mode = "copy"`), so a skill exists once on disk. Anything skd moves out of the way lands in `<library>/.trash/`, listed by `skd trash`: a link is only removed outright when it points into the library (at the place the lock records) or is exactly a link skd's own record says it made, a copy only when that record says skd wrote exactly it (a link or copy made before skd kept the record is recorded by the next `skd apply` while it is exactly what skd would make: a link to the library or to the app's directory, a copy identical to its source with nothing added), and a vendored directory only when it matches the lock. A link skd moves aside (one of yours, with `--force`, or a broken one `adopt` clears) is listed too, but you move it back by hand. Links an older skd made, before it kept its record, are still treated as it treated them on the first command after upgrading: an app's link that is exactly a link to the absolute directory `skills.lock` records for it (a skill, not in an agent's directory) is removed when the app is disabled or taken out of `skills.toml`; after the library is moved (`mv`), a link to its old location is removed when it leads nowhere because that location is gone from a folder that is still there (not when a volume is unmounted). `skd trash restore` only puts a directory back where skd takes them from, an entry of an agent's skill directory (for an approved custom target, only an entry named like a skill, so never `.ssh`) or a place inside the library's `vendor/`; a trash entry recorded as coming from anywhere else, or reached through a symbolic link, is listed as not restorable with the reason, and is left for you to move by hand (a library cloned from someone else carries their `.trash/` too). `skd remove` of a pack's last skill drops the pack.

## Unattended updates

```cron
0 4 * * * cd ~/.skilldots && skd update --yes --commit   # --yes: no question, every pack
```

Or just check, and update by hand when `skd list` says so:

```cron
0 4 * * * skd outdated >/dev/null 2>&1
```

`--commit` commits and pushes the library when something changed, so other machines pick it up with `git pull && skd apply`.

## Status

Early. Linking, vendoring, updates, per-agent control, scan/adopt and the trash work; Windows copy-mode hardening and Homebrew/scoop packages are next. See the [design notes](docs/design.md).

## License

MIT
