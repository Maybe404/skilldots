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
- Nothing is deleted. `remove`, `purge` and `apply --force` move files into the library's trash; `skd trash` lists and restores them. `skd trash empty` is the one irreversible command and it only runs for a person at a terminal, so an agent can do everything short of permanent deletion.
- People get a full-screen dashboard (`skd` with no arguments) that does everything the commands do, for looking around and vetoing.

## Install

```sh
go install github.com/Maybe404/skilldots/cmd/skd@latest
```

Prebuilt binaries will follow.

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
| `skd add <owner/repo \| url \| "pasted install line">[@ref]` | fetch a repo and choose skills interactively, or with `--skill`, `--category`, `--all` (`--all` follows everything; names given with it are only checked). On a pack that follows `"*"`, `--skill x` takes `x` out of `skip`; if a wildcard such as `eng/*` excludes it, the command fails and tells you to edit `skills.toml`. Also takes a whole `npx skills add owner/repo --skill x` line (bunx, pnpx, `pnpm dlx`, `yarn dlx`, `npm exec`, `npx add-skill`, `skd add`; quote it), a skills.sh skill page, or a GitHub link to a skill directory (`/tree/<ref>/<dir>`) or its `SKILL.md` (`/blob/<ref>/…`): the repository is taken from it and the skills it names are ticked in the picker (without a terminal they are the selection). The text is only parsed, never run; unquoted, only an install command may span several arguments, and a source or `@ref` starting with `-` is refused. The `<ref>` in a GitHub link is where the page was browsed and is not pinned; append `@ref` to pin. With no source at a terminal it asks for one. |
| `skd list [pack] [--all] [--flat] [--agent x]` | overview; expand a pack; flat list; what one agent receives |
| `skd apply [-n] [--force]` | link/unlink to match the manifest; `-n` previews |
| `skd update [pack…] [--commit]` | at a terminal: check upstream, list each pack's changes (changed / new / gone / moved), every pack ticked; only the ticked packs are updated (ctrl+c cancels). Named packs with nothing new upstream are offered an "update anyway", which re-vendors missing skills and re-links (as `u` in the dashboard); with none named it points to `skd apply`. With `--yes`, `--json` or no terminal it updates straight away, every pack when none are named. Then re-vendors the selected skills, reports, and records the update |
| `skd history [pack] [-n N]` | what each update changed: when, old → new commit, the skills changed / added / moved / gone / new upstream, and the subjects of up to 20 upstream commits that touched them. Read from `skills.history.jsonl` in the library (committed and pushed with it), so it works offline and on another machine after a clone |
| `skd outdated [pack…]` | check upstream without changing anything: per pack, the selected skills that changed (a skill only moved to another directory is listed as `moved` and is not an update: a pack with nothing but moves is not counted and `U` in the dashboard skips it, though `u` or `skd update` on it still renames the vendored directory to the new path), skills new upstream since the last update (or since `add`, for a pack never updated), skills gone upstream; exits non-zero if a pack could not be fetched. `skd list` and the menu show the last result (`↑ 2 updates available · checked 2h ago`, stale after a day) without fetching |
| `skd enable <skill\|pack> [--agent a,b]` | link into agents (default set when omitted) |
| `skd disable <skill\|pack> [--agent a,b]` | unlink from agents (all when omitted); the library keeps it |
| `skd remove <skill>` | deselect a vendored skill: unlinked everywhere, dropped from `vendor/` |
| `skd purge <pack>` | delete a pack from manifest, lock, `vendor/` and every agent directory |
| `skd fork <skill> [--private] [--as name]` | copy a vendored skill into `skills/` to edit it; upstream no longer touches it |
| `skd path <skill>` | print where a skill's files live, for editing |
| `skd edit <skill> [--fork [--private] [--as name] \| --force]` | open a skill's directory in your editor: the `editor` setting, `$VISUAL`, `$EDITOR`, then VS Code, Cursor, Zed or Sublime Text (command or macOS app), then the system opener. A vendored or app skill is replaced on update, so at a terminal skd offers to fork it first; without a terminal it refuses unless `--fork` (fork, open the copy) or `--force` (open the files anyway) is given. A terminal editor (vim, nano) needs a terminal; when the editor cannot run (not installed, or a terminal editor without a terminal) `--fork` fails before forking anything |
| `skd config list \| get <key> \| set <key> [value]` | settings, per user and machine: `editor` (command), `theme` (`light`, `dark`, `auto`), `lang` (reserved; only English so far); `library` is shown and set by `skd use`. No value unsets |
| `skd new <name> [--private]` | scaffold a skill of your own and link it; the name follows the skill name rule below |
| `skd scan` | inventory every agent skill directory: managed, copies, links, broken |
| `skd adopt [-n] [--yes]` | take over unmanaged entries: subscribe, register apps, copy as your own, enable per agent |
| `skd trash [restore <id> \| empty]` | what skd set aside; restore it; or (interactively) delete for good |
| `skd targets` | known agent directories and which exist on this machine |
| `skd clone <git-url> [dir]` | set up a machine from a library you pushed elsewhere |
| `skd sync [--remote url] [-m msg]` | commit the library and push it: your backup |
| `skd use <dir>` | point skd at an existing library from any directory |
| `skd [--simple]` | with no arguments: the full-screen dashboard (below); `--simple`, a narrow terminal or `TERM=dumb` give a question-and-answer menu instead |

All commands take `--json` for scripts and agents, `--yes` to skip prompts, and `--theme light|dark|auto` for colours (default `auto`, also `$SKILLDOTS_THEME`, then `skd config set theme`, where a value skd does not know falls back to `auto` with a warning; `NO_COLOR` turns colour off). Under `--json` stdout carries exactly one JSON document (progress and warnings go to stderr); a failing command prints nothing on stdout and exits non-zero, except `skd update`, which reports `{packs, failed, apply, committed, history}` and exits non-zero when any pack failed, and `skd outdated`, which does the same with `{checked, packs, failed, updates}` (each pack: `repo, commit, latest, changed, moved, new, gone, pending, checked`). Problems confined to one skill — an unknown target, a name clash, a pack not fetched yet — are warnings, not errors: the rest of the library keeps working.

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
| `s` | back up: commit and push | `sync` |
| `A` / `r` | apply pending link changes / reload | `apply` |
| `,` | settings: editor, theme (applies next start) | `config` |
| `?` | every key | |
| `q` `esc` | quit; `ctrl+c` while something runs quits when it ends, twice stops it | |

While an operation runs, keys that would start another are refused; problems show on the status line.

**Skills Claude Code turned off.** Claude Code keeps skills a person turned off inside it in `skillOverrides` of `~/.claude/settings.json` (`$CLAUDE_CONFIG_DIR/settings.json` when set). skd reads that file — never writes it, and ignores it when missing or malformed — and marks those skills `off in Claude Code` in `skd list` (`ClaudeDisabled` in `--json`) and the dashboard. skd still links them; to keep one out of Claude entirely, remove the claude target (`skd disable <skill> --agent claude`, or `e` in the dashboard).

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

## How skills are found in a repository

Every directory containing a `SKILL.md` is a skill. Test directories are skipped, and so are hidden directories (except `.agents/`, the cross-agent convention) — unless the repository has skills nowhere else, as when it ships them only in `.claude/skills/`. Repositories that copy the same skill into many agent folders are collapsed to one, preferring `skills/` or `.agents/skills/`; `skd show` lists the other copies whose content differs.

The category is the skill's parent path with structural segments (`skills`, `skill`, `plugins`, `plugin`, hidden directories) removed: `skills/engineering/tdd` is `engineering`, `plugins/code-improver/skills/pr-improver` is `code-improver`. It depends only on that skill's own location, so new directories upstream never recategorise existing skills; when a skill does move, its vendored directory is renamed.

A skill's name (frontmatter `name`, else its directory) must be letters, digits, `.`, `_` and `-`, not starting with `.`. Skills with any other name are skipped with a warning, because the name becomes a directory and link name. The same rule applies to `as`, `[[app]].name` and `[[local]].name`. Within one repository, a skill whose name differs only in case from another, or whose category would put it inside another skill's directory, is skipped with a warning. A `LICENSE` that is a symlink is never copied.

## Name clashes

The first skill to use a name keeps it. When `skd add` or `skd update` vendors a skill whose name is already taken, it is linked as `<repo>-<name>` and the `as` is written to `skills.toml`. Names are compared case-insensitively. Two skills may share a name if they go to different agents (commands then need a qualified name: `owner/repo:foo` or `<alias>:foo`, `skills:foo`, `private:foo`, `app:foo`, and fail listing the candidates otherwise); if they would land in the same agent directory (for example you create `skills/foo` while a pack provides `foo` to the same agents), both are skipped with a warning naming both directories, existing links stay as they are, and everything else is linked as usual. `skd fork` renames the frontmatter to the fork's name, so a fork never clashes with its upstream.

## Editing, rolling back, removing

- **Third-party skill you want to change:** `skd fork <skill>`. The copy in `skills/` is yours; the vendored original is deselected so the names do not clash.
- **Your own skill:** `skd edit <skill>` (or `o` in the dashboard) opens it; edit in place and commit.
- **What did an update change?** `skd history [pack]`: per update, the skills that changed and upstream's commit messages for them.
- **Roll back an update:** the library is a git repository. With `skd update --commit` every update is one commit, so `git revert <commit> && skd apply` undoes it. Without `--commit`, `git checkout -- vendor skills.lock && skd apply`.
- **Remove:** `skd disable` unlinks but keeps the files; `skd remove` also drops a vendored skill from the library; `skd purge` deletes a whole pack.

- **Moved the library:** `skd use <new dir>` then `skd apply`; links into the old location are recognised and re-pointed.

Agent directories only ever contain symlinks into the library (copies on Windows without symlink rights, or for targets with `mode = "copy"`), so a skill exists once on disk. Anything skd moves out of the way lands in `<library>/.trash/`, listed by `skd trash`: a copy is only discarded when it is exactly what skd wrote, and a vendored directory only when it matches the lock. `skd remove` of a pack's last skill drops the pack.

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
