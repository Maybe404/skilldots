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
| `skd init [dir]` | create a library; default targets are detected from installed agents |
| `skd add <owner/repo \| url>[@ref]` | fetch a repo and choose skills interactively, or with `--skill`, `--category`, `--all` |
| `skd list [pack] [--all] [--flat] [--agent x]` | overview; expand a pack; flat list; what one agent receives |
| `skd apply [-n] [--force]` | link/unlink to match the manifest; `-n` previews |
| `skd update [pack…] [--commit]` | fetch upstream, re-vendor selected skills, report changed / new / gone |
| `skd enable <skill\|pack> [--agent a,b]` | link into agents (default set when omitted) |
| `skd disable <skill\|pack> [--agent a,b]` | unlink from agents (all when omitted); the library keeps it |
| `skd remove <skill>` | deselect a vendored skill: unlinked everywhere, dropped from `vendor/` |
| `skd purge <pack>` | delete a pack from manifest, lock, `vendor/` and every agent directory |
| `skd new <name> [--private]` | scaffold a skill of your own and link it |
| `skd targets` | known agent directories and which exist on this machine |

All commands take `--json` for scripts and agents, and `--yes` to skip prompts.

## skills.toml

```toml
[targets]
default = ["agents", "claude", "codex"]

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

## Unattended updates

```cron
0 4 * * * cd ~/.skilldots && skd update --yes --commit
```

`--commit` commits and pushes the library when something changed, so other machines pick it up with `git pull && skd apply`.

## Status

Early. Linking, vendoring, updates and per-agent control work; `scan`/`adopt` for taking over skills installed by other tools, Windows copy-mode fallback hardening, and Homebrew/scoop packages are next. See the [design notes](docs/design.md).

## License

MIT
