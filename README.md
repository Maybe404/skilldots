# skilldots

Declarative skill management for AI coding agents. One git-backed library, linked into every agent you use.

```
skd add mattpocock/skills          # pick skills from a repo; vendored into your library
skd add trailofbits/skills -s differential-review
skd list                           # one line per repo, not one per skill
skd update                         # fetch upstream, show what changed, re-link
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
- Every command works without a terminal: `--json` for structured results, `--yes` for prompts, `skd show owner/repo --json` to inspect a repository before `skd add --skill …`.
- Destructive steps (`apply --force`, `purge`) are the only ones that need a human; the skill tells agents to preview and ask.
- People get the interactive menu (`skd` with no arguments) for looking around and vetoing.

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
├── skills/<name>/                # skills you write; pushed with the library
├── private/<name>/               # skills for this machine only; its own git repo, git-ignored here
└── vendor/<owner>/<repo>/[category/]<name>/   # third-party, written by skd
```

## Commands

| | |
|---|---|
| `skd init [dir]` | create a library; the default target is `agents` (`~/.agents/skills`), which nearly every agent reads |
| `skd show <owner/repo \| url>` | list what a repository offers, without installing |
| `skd add <owner/repo \| url>[@ref]` | fetch a repo and choose skills interactively, or with `--skill`, `--category`, `--all` |
| `skd list [pack] [--all] [--flat] [--agent x]` | overview; expand a pack; flat list; what one agent receives |
| `skd apply [-n] [--force]` | link/unlink to match the manifest; `-n` previews |
| `skd update [pack…] [--commit]` | fetch upstream, re-vendor selected skills, report changed / new / gone |
| `skd enable <skill\|pack> [--agent a,b]` | link into agents (default set when omitted) |
| `skd disable <skill\|pack> [--agent a,b]` | unlink from agents (all when omitted); the library keeps it |
| `skd remove <skill>` | deselect a vendored skill: unlinked everywhere, dropped from `vendor/` |
| `skd purge <pack>` | delete a pack from manifest, lock, `vendor/` and every agent directory |
| `skd fork <skill> [--private] [--as name]` | copy a vendored skill into `skills/` to edit it; upstream no longer touches it |
| `skd path <skill>` | print where a skill's files live, for editing |
| `skd new <name> [--private]` | scaffold a skill of your own and link it |
| `skd targets` | known agent directories and which exist on this machine |
| `skd clone <git-url> [dir]` | set up a machine from a library you pushed elsewhere |
| `skd sync [--remote url] [-m msg]` | commit the library and push it: your backup |
| `skd use <dir>` | point skd at an existing library from any directory |
| `skd` | with no arguments: an interactive menu for all of the above |

All commands take `--json` for scripts and agents, and `--yes` to skip prompts.

## skills.toml

```toml
[targets]
default = ["agents"]                         # add agents that do not read ~/.agents/skills

[targets.custom.myagent]
path = "~/.myagent/skills"

[[pack]]
repo = "mattpocock/skills"
alias = "matt"
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

Every directory containing a `SKILL.md` is a skill. Hidden and test directories are skipped (except `.agents/`, the cross-agent convention). Repositories that copy the same skill into many agent folders are collapsed to one. Categories come from the path with structural segments (`skills`, `plugins`) removed, so `skills/engineering/tdd` is category `engineering` and `plugins/x/skills/x` is uncategorised.

## Editing, rolling back, removing

- **Third-party skill you want to change:** `skd fork <skill>`. The copy in `skills/` is yours; the vendored original is deselected so the names do not clash.
- **Your own skill:** edit it in place (`skd path <skill>` prints the directory) and commit.
- **Roll back an update:** the library is a git repository. With `skd update --commit` every update is one commit, so `git revert <commit> && skd apply` undoes it. Without `--commit`, `git checkout -- vendor skills.lock && skd apply`.
- **Remove:** `skd disable` unlinks but keeps the files; `skd remove` also drops a vendored skill from the library; `skd purge` deletes a whole pack.

Agent directories only ever contain symlinks into the library (copies on Windows without symlink rights), so a skill exists once on disk.

## Unattended updates

```cron
0 4 * * * cd ~/.skilldots && skd update --yes --commit
```

`--commit` commits and pushes the library when something changed, so other machines pick it up with `git pull && skd apply`.

## Status

Early. Linking, vendoring, updates and per-agent control work; `scan`/`adopt` for taking over skills installed by other tools, Windows copy-mode fallback hardening, and Homebrew/scoop packages are next. See the [design notes](docs/design.md).

## License

MIT
