# skilldots

Declarative skill management for AI coding agents. One git-backed library, linked into every agent you use.

Claude Code, Codex, Cursor and the rest each read skills from their own folder. skilldots keeps every skill you use in one library (a git repository), links it into each agent's folder, and lets you see, update, roll back and remove them from one place: the `skd` command, a terminal dashboard, or a Mac app. Your agents can drive it too.

**Try the desktop app's interface:** [skilldots.maybe404.com](https://skilldots.maybe404.com) runs it in your browser with sample data; nothing is installed or written.

## Install

**Mac.** The desktop app brings `skd` with it:

```sh
curl -fsSL https://skilldots.maybe404.com/install.sh | sh
```

The script installs skilldots.app (Apple silicon and Intel) from the latest [release](https://github.com/Maybe404/skilldots/releases), checks its SHA-256, links `/usr/local/bin/skd` to the app (asking for your password only when that directory needs it), and, when run from a terminal, opens the app. On first launch the app creates your library and installs the `skilldots` skill for every agent it finds, so Claude Code, Codex and the rest can manage skills through `skd` right away. That skill comes with the app (no internet needed, always matching the app's `skd`); an `skd` installed with `go install` follows it from GitHub instead. Settings → Agents installs, repairs or removes either later.

Prefer the disk image from the release page? The app is signed but not notarized: copy it to Applications, open it once, then choose **Open Anyway** in System Settings → Privacy & Security.

**Anywhere else, or for the command line only:**

```sh
go install github.com/Maybe404/skilldots/cmd/skd@latest
```

skd runs `git` for everything it fetches and commits; git 2.24 or later is best ([why](docs/safety.md#git)).

## Quick start

```sh
skd init                           # create ~/.skilldots and teach your agents to use skd
skd add mattpocock/skills          # pick skills from a repo; vendored into your library
skd add trailofbits/skills -s differential-review
skd add "npx skills add vercel-labs/agent-skills --skill web-design-guidelines"   # pasted install line
skd edit tdd                       # open a skill in your editor
skd list                           # one line per repo, not one per skill
skd update                         # show what changed upstream, update the packs you tick
skd history                        # what each update changed, with upstream's commit messages
skd disable grill-me --agent codex # per-skill, per-agent control
skd purge mattpocock/skills        # gone from the library and every agent dir
```

`skd` with no arguments opens a full-screen [dashboard](#dashboard) that does all of this with keys. Already have skills scattered across agent folders? `skd scan` lists them and `skd adopt` takes them over.

## Why

Skills are just folders with a `SKILL.md`, and every agent reads them from its own directory (`~/.claude/skills`, `~/.codex/skills`, `~/.agents/skills`, …). Installers copy files into those directories and then forget about them: you lose track of what came from where, updates overwrite silently, and removing a skill means hunting through a dozen folders.

skilldots treats skills like dotfiles:

- **`skills.toml` is the source of truth.** It says which repositories you follow, which skills you picked, and which agents get them. `skd apply` makes the machine match it.
- **The library is a git repository.** Third-party skills are vendored in, so an upstream change shows up as a diff you can read and revert. A new machine is `git clone` + `skd apply`.
- **Repositories are the unit, skills are the selection.** A repo with 40 skills is one line in `skd list` and one `git fetch` in `skd update`, however many of its skills you use.
- **Your own skills live alongside.** `skills/` is for ones you publish with the library; `private/` is a separate local git repo that is never pushed.
- **Every link is recorded.** `skd` only ever touches files it placed itself, and tells you when something else is in the way.

## Built for agents

Most of the time it is not a person running `skd` but the agent on their machine. skilldots is designed around that:

- **Agents learn skd.** `skd init` (and a new library in the desktop app) installs the [`skilldots` skill](skills/skilldots/SKILL.md) for every agent on the machine: through `~/.agents/skills`, which most agents read, and into the own directory of each installed agent that does not (Claude Code, Kiro, …), so no agent loads it twice. Each agent then knows how to add, update, fork and remove skills through `skd` instead of poking at directories.
- **No terminal needed.** Every command works when stdin or stdout is not a terminal, so `skd update | cat` never waits on a hidden prompt. Use `--json` for structured results, `--yes` for prompts, and `skd show owner/repo --json` to inspect a repository before `skd add --skill …`.
- **Nothing is deleted.** `remove`, `purge` and `apply --force` move files into the library's trash; `skd trash` lists and restores them. skd removes outright only what it can tell it made. The exact rules are in [docs/safety.md](docs/safety.md#the-trash).
- **Permanent deletion is for people.** `skd trash empty` is the one irreversible command, and it only runs for a person at a terminal (in the desktop app, the Trash page deletes for good after a confirmation that names what goes): stdin and stdout are terminals and, where it asks, the answer was typed. Approving a [custom target](#custom-targets) works the same way. This keeps out agents' ordinary tool calls, scripts, cron and `--yes`, so an agent can do everything short of permanent deletion.
- **One honest gap.** A program that drives a pseudo-terminal itself (`script`, `expect`, a terminal it controls) can answer like a person. skd cannot tell the difference; the bundled skill forbids agents to do that.
- **Parse `--json`, never the words.** The text meant for people may be in Chinese (see [Language](#language)); `--json` is not, whatever the language.
- **People can watch and veto.** The full-screen dashboard (`skd` with no arguments) does everything the commands do.

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

One line each; [docs/commands.md](docs/commands.md) has every option and rule.

| | |
|---|---|
| **Library** | |
| `skd init [dir]` | create a library (its own git repository) and teach your agents to use skd |
| `skd clone <git-url> [dir]` | set up a machine from a library you pushed elsewhere |
| `skd use <dir>` | point skd at an existing library from any directory |
| `skd sync [--remote url] [-m msg]` | commit the library and push it: your backup |
| **Adding** | |
| `skd show <owner/repo \| url>` | list what a repository offers, without installing |
| `skd add <source>[@ref]` | fetch a repo and choose skills, interactively or with `--skill`, `--category`, `--all`; also takes a pasted install line, a skills.sh page or a GitHub link (only parsed, never run) |
| **Looking** | |
| `skd list [pack] [--all] [--flat] [--agent x]` | overview; expand a pack; flat list; what one agent receives |
| `skd path <skill>` | print where a skill's files live |
| `skd scan` | inventory every agent skill directory: managed, copies, links, broken, left |
| **Linking** | |
| `skd apply [-n] [--force]` | link and unlink to match the manifest; `-n` previews; `--force` moves what is in the way to the trash |
| `skd enable <skill\|pack> [--agent a,b]` | link into agents (the default set when omitted) |
| `skd disable <skill\|pack> [--agent a,b]` | unlink from agents (all when omitted); the library keeps it |
| `skd targets` | known agent directories, which exist here, and unused custom targets with why |
| `skd targets trust <name>` / `untrust <name>` | approve a custom target (a person at a terminal only) / withdraw that |
| `skd targets default <agent>...` | the agents a skill without its own list is linked into |
| `skd targets pause <agent>...` / `resume <agent>...` | take an agent's links out for a while, then put back exactly what was there |
| **Updating** | |
| `skd outdated [pack…]` | check upstream without changing anything |
| `skd update [pack…] [--commit]` | show what changed upstream and update the packs you tick |
| `skd history [pack] [-n N]` | what each update changed, with upstream's commit messages |
| **Changing** | |
| `skd new <name> [--private]` | scaffold a skill of your own and link it |
| `skd fork <skill> [--private] [--as name]` | copy a vendored skill into `skills/` to edit it; upstream no longer touches it |
| `skd edit <skill> [--fork \| --force]` | open a skill in your editor; a vendored one is offered a fork first |
| `skd remove <skill>` | unlink a skill everywhere and move its directory to the trash |
| `skd purge <pack>` | delete a pack from the manifest, the lock, `vendor/` and every agent directory |
| **Cleaning up** | |
| `skd adopt [-n] [--yes]` | take over unmanaged entries: subscribe, register apps, copy as your own (always, when your copy differs from upstream), enable per agent |
| `skd trash [restore <id\|skill> \| empty]` | what skd set aside and why; restore it (undoing `remove` or `purge`, links included); or (a person, interactively) delete for good |
| `skd config list \| get <key> \| set <key> [value]` | settings: `editor`, `editor-kind`, `theme`, `lang`, `library` |
| `skd [--simple]` | the dashboard; `--simple`, a narrow terminal or `TERM=dumb` give a question-and-answer menu |

Every command takes `--json` for scripts and agents, `--yes` to skip prompts, `--lang en|zh|auto` for the [language](#language) and `--theme light|dark|auto` for colours. Under `--json`, stdout carries exactly one JSON document and progress goes to stderr; [docs/commands.md](docs/commands.md#what---json-prints) lists the exit codes and the documents of `update`, `outdated` and `apply`. Problems confined to one skill (an unknown target, a name clash, a pack not fetched yet) are warnings, not errors: the rest of the library keeps working.

## Dashboard

`skd` in a terminal at least 90 columns wide opens a dashboard: packs and your own skills on the left, the selected pack's skills on the right. It runs the same code as the commands, so anything you do there leaves the library exactly as the matching command would. In every pick or list dialog, `/` filters by name, description or category.

| key | does | command |
|---|---|---|
| `↑↓` `j k` `pgup pgdn` `g G` | move | |
| `tab` `← →` `l` `enter` | switch column, open a pack | |
| `/` | filter the column (`esc` clears) | |
| `space` | enable / disable the skill; installs an upstream one | `enable`, `disable`, `add --skill` |
| `a` | add a repository (or paste an install line or a skills.sh / GitHub link: what it names is ticked): tick skills or whole categories (or `*` for everything), then an optional alias and agents | `add [--category] [--alias] [--agent]` |
| `u` / `U` | update this pack / every pack with upstream changes. Both check upstream first and show what would change, skill by skill; `u` asks to confirm, `U` lists the packs ticked and updates the ones you leave ticked | `update` |
| `h` | update history: each update, and upstream's commit messages for it | `history` |
| `c` | check upstream again (see below) | `outdated` |
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
| `,` | settings: editor, editor kind, theme (applies next start), language (applies at once) | `config` |
| `?` | every key | |
| `q` `esc` | quit; `ctrl+c` while something runs quits when it ends, twice stops it | |

- **Update checks.** The dashboard checks upstream once in the background when it opens. It shows `↑ N packs have updates` in the header, `↑` on the packs, and `↑ changed`, `new upstream`, `gone upstream` (and `moved upstream`, which is not an update) on the skills.
- **One thing at a time.** While an operation runs, keys that would start another are refused; problems show on the status line.
- **Colours follow the terminal.** skd asks the terminal for its background colour and uses a light or dark palette for the dashboard, the menu and every table. It gives up after 300 ms and assumes dark; it does not ask when you have already typed ahead, and keys typed while it waits are passed on to the dashboard. `--theme light` or `--theme dark` (or `SKILLDOTS_THEME`) skips the question.

### Skills Claude Code turned off

Claude Code keeps the skills a person turned off inside it in `skillOverrides` of `~/.claude/settings.json` (`$CLAUDE_CONFIG_DIR/settings.json` when set).

- skd reads that file but never writes it, and ignores it when it is missing or malformed.
- It marks those skills `off in Claude Code` in `skd list` (`ClaudeDisabled` in `--json`) and the dashboard. They are matched by link name, the name Claude sees, and only for skills linked into Claude's directory.
- skd still links them. To keep one out of Claude entirely, remove the claude target: `skd disable <skill> --agent claude`, or `e` in the dashboard.

## Language

skd speaks English and Simplified Chinese (简体中文): command help, tables, progress, prompts, errors and the whole dashboard. The language is chosen, first match wins, by

1. `--lang en|zh|auto` on the command line,
2. the environment variable `SKILLDOTS_LANG` (`en`, `zh` or `auto`),
3. the setting: `skd config set lang zh` (`en`, `zh`, `auto`; no value unsets it),
4. `auto`, which follows the system: the first of `LC_ALL`, `LC_MESSAGES` and `LANG` that is set (`zh_CN.UTF-8`, `zh_TW.UTF-8` … are Chinese, anything else, `C` too, English); when none is set, on macOS the preferred languages of the system (`defaults read -g AppleLanguages`); English when nothing can be told.

A value that is not `auto`, `en` or `zh` in `--lang` or `$SKILLDOTS_LANG` is refused like a bad `--theme`; a bad `lang` setting only warns once on stderr and falls back to `auto`. In the dashboard, `,` opens the settings: changing `lang` there switches the running dashboard at once. An explicit choice there wins over `$SKILLDOTS_LANG` until the next start, and the status line says so.

**`--json` does not change with the language.** Under `--json` every message is English, on stdout and on stderr: the document, the `reason`/`problem`/`error` fields in it and the progress lines around it are byte for byte what they are with `--lang en`, so scripts and agents can parse them (or match on the words) in any language. What skd writes to files (`skills.toml` comments, `skills.lock`, the update history, commit messages, the skill scaffold) is English too. The Chinese terms are listed in [docs/i18n.md](docs/i18n.md).

## skills.toml

```toml
[targets]
default = ["agents", "claude"]               # a new library: agents, plus claude when Claude Code is installed
disabled = []                                # paused agents (skd targets pause): no links, selections kept

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
name = "release-notes"
targets = ["agents"]
```

### Targets

- **Defaults.** A new library links into `agents` (`~/.agents/skills`, which nearly every agent reads) and, when Claude Code is installed, `claude`, because Claude Code does not read `~/.agents/skills`. On a machine without Claude Code, no `~/.claude` is made.
- **Inheritance.** Targets inherit: global default → pack → skill. `targets = []` keeps a skill in the library without linking it anywhere.
- **Agents that are not installed.** skd does not create the skills directory of a built-in agent that is not installed on the machine (the shared `~/.agents/skills` excepted): `skd apply` skips that agent and says so.
- **`"*"`** in a targets list stands for the default targets plus every built-in agent installed on the machine resolving the library (its skill directory, or the directory holding it, exists). Each machine links into the agents it has, and an agent installed later gets the skill at the next `skd apply`. When `agents` is among the targets, `"*"` leaves out the agents that read `~/.agents/skills` themselves (Codex, Gemini CLI, Cursor, Copilot, …; `skd targets` marks them), so no skill is loaded twice; agents named explicitly are kept.
- **The `skilldots` skill** is followed with the default targets of the day plus `"*"`: from the copy inside skilldots.app (an `[[app]]` entry) when the app installed it, from `Maybe404/skilldots` on GitHub otherwise. An older skd ignores `"*"` with a warning and keeps linking the defaults; a later change of `targets.default` does not reach it. Editing a skill's agents (`skd enable`, `skd disable`, the app) turns `"*"` into the names it stands for.

### Application skills

An `[[app]]` is linked (or copied) only when its `path` is a skill: a directory holding `SKILL.md` as a plain file, named by an absolute path (or one starting with `~/`). Anything else, and anything in or around a place of keys and credentials such as `~/.ssh`, is skipped with a warning; see [docs/safety.md](docs/safety.md#application-skills-app).

### Custom targets

**Custom targets need your approval on each machine.** A `skills.toml` may come from someone else's library (`skd clone`), and skd writes into its targets. So a `[targets.custom.<name>]` is used only after a person approves it at a terminal, with `skd targets trust <name>` or `T` in the dashboard, which show the directory it leads to first. Until then it is ignored everywhere, with a warning saying why.

The approval covers that name, directory and mode, and is kept on your machine, never in the library. Some directories are refused even with approval: anything outside your home, places that hold keys or files that are loaded or run by themselves (shell and editor configuration, autostart folders, `bin` directories), and anything around or inside a built-in agent's directory, among others. The full rules are in [docs/safety.md](docs/safety.md#custom-targets).

## How skills are found in a repository

- Every directory containing a `SKILL.md` is a skill.
- Test directories are skipped, and so are hidden directories (except `.agents/`, the cross-agent convention) — unless the repository has skills nowhere else, as when it ships them only in `.claude/skills/`.
- Repositories that copy the same skill into many agent folders are collapsed to one, preferring `skills/` or `.agents/skills/`; `skd show` lists the other copies whose content differs.

**Categories.** The category is the skill's parent path with structural segments (`skills`, `skill`, `plugins`, `plugin`, hidden directories) removed: `skills/engineering/tdd` is `engineering`, `plugins/code-improver/skills/pr-improver` is `code-improver`. It depends only on that skill's own location, so new directories upstream never recategorise existing skills; when a skill does move, its vendored directory is renamed.

**Names.** A skill's name (frontmatter `name`, else its directory) must be letters, digits, `.`, `_` and `-`, not starting with `.`. Skills with any other name are skipped with a warning, because the name becomes a directory and link name. The same rule applies to `as`, `[[app]].name` and `[[local]].name`. Within one repository, a skill whose name differs only in case from another, or whose category would put it inside another skill's directory, is skipped with a warning.

**Links.** A `LICENSE` that is a symlink is never copied, and nothing reached through a symbolic link inside the library is linked or copied into an agent ([details](docs/safety.md#symbolic-links-inside-the-library)). The library itself may be reached through a link.

## Name clashes

- The first skill to use a name keeps it. When `skd add` or `skd update` vendors a skill whose name is already taken, it is linked as `<repo>-<name>` and the `as` is written to `skills.toml`.
- Names are compared case-insensitively.
- Two skills may share a name if they go to different agents. Commands then need a qualified name (`owner/repo:foo` or `<alias>:foo`, `skills:foo`, `private:foo`, `app:foo`) and fail listing the candidates otherwise.
- If they would land in the same agent directory (for example you create `skills/foo` while a pack provides `foo` to the same agents), both are skipped with a warning naming both directories. Existing links stay as they are, and everything else is linked as usual.
- `skd fork` renames the frontmatter to the fork's name, so a fork never clashes with its upstream.

## Editing, rolling back, removing

- **Third-party skill you want to change:** `skd fork <skill>`. The copy in `skills/` is yours; the vendored original is deselected so the names do not clash.
- **Your own skill:** `skd edit <skill>` (or `o` in the dashboard) opens it; edit in place and commit.
- **What did an update change?** `skd history [pack]`: per update, the skills that changed and upstream's commit messages for them.
- **Roll back an update:** the library is a git repository. With `skd update --commit` every update is one commit, so `git revert <commit> && skd apply` undoes it. Without `--commit`, `git checkout -- vendor skills.lock && skd apply`.
- **Remove:** `skd disable` unlinks but keeps the files; `skd remove` also moves the skill's directory (vendored or your own) to the library trash; `skd purge` deletes a whole pack. `skd remove` of a pack's last skill drops the pack.
- **Stop using an agent for a while:** `skd targets pause <agent>`; `skd targets resume <agent>` brings back every link.
- **Moved the library:** `skd use <new dir>` then `skd apply`; links into the old location are recognised and re-pointed. One skd recorded making, or one that leads nowhere now, is removed outright; any other goes to the trash, or, for a skill no longer wanted, stays where it is.

Agent directories only ever contain symlinks into the library (copies on Windows without symlink rights, or for targets with `mode = "copy"`), so a skill exists once on disk. Anything skd moves out of the way lands in `<library>/.trash/`, listed by `skd trash`. What it removes outright, what `skd trash restore` puts back, and how links made by an older skd are handled: [docs/safety.md](docs/safety.md#the-trash).

## Unattended updates

```cron
0 4 * * * cd ~/.skilldots && skd update --yes --commit   # --yes: no question, every pack
```

Or just check, and update by hand when `skd list` says so:

```cron
0 4 * * * skd outdated >/dev/null 2>&1
```

`--commit` commits and pushes the library when something changed, so other machines pick it up with `git pull && skd apply`.

## Safety

skd is built to be run by agents on libraries that may come from someone else. In short:

- Nothing is deleted for good without a person: at a terminal (`skd trash empty`), or confirming it in the desktop app's Trash page. skd removes outright only links and copies it can tell it made; anything else in the way goes to the trash.
- Custom targets need your approval on each machine, and some directories are refused even then.
- Nothing skd prints for a person carries control characters or bidi controls from a repository, a library or an agent directory, so a hostile `SKILL.md` cannot rewrite your terminal.

[docs/safety.md](docs/safety.md) has every rule.

## Status

Early. Linking, vendoring, updates, per-agent control, scan/adopt and the trash work; Windows copy-mode hardening and Homebrew/scoop packages are next. See the [design notes](docs/design.md).

## License

MIT
