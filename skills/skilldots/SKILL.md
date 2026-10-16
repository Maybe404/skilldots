---
name: skilldots
description: Manage the skills installed for AI agents on this machine with the `skd` CLI (skilldots). Use when the user asks to install, add, find, update, remove, enable, disable, fork, list, back up or sync agent skills, or mentions skills.toml, a skill library, or a GitHub repository of skills. Also use before editing anything under ~/.agents/skills or ~/.claude/skills, because those entries are links managed by skd.
license: MIT
---

# skilldots (`skd`)

`skd` keeps every skill in one git-backed **library** and links it into each agent's skill directory (`~/.agents/skills`, `~/.claude/skills`, …). The manifest `skills.toml` is the source of truth; never edit the agent directories or `vendor/` by hand.

Run `skd --help` for the full command list. Every command accepts `--json` (structured output) and `--yes` (no prompts). Prefer `--json` when you need to read results.

## Find the library

```bash
skd list --json        # fails with a clear message if there is no library yet
```

No library: ask the user whether to create one (`skd init`), clone their backup (`skd clone <git-url>`), or point at an existing directory (`skd use <dir>`). Do not pick for them.

## Install a skill from a repository

1. Look before installing — repositories often hold dozens of skills:
   ```bash
   skd show owner/repo --json
   ```
2. Install only what the task needs, by name:
   ```bash
   skd add owner/repo --skill <name>[,<name>…]
   skd add owner/repo --category <category>   # everything in one category
   skd add owner/repo --all                   # follow the whole repo, including future skills
   ```
   Never run `skd add` without `--skill`, `--category` or `--all`: it opens an interactive picker meant for humans.
3. `add` vendors the files into the library and links them. If a name clashes with an existing skill, skd links it as `<repo>-<name>` and says so; tell the user.

Sources: `owner/repo`, a GitHub URL, any git URL, optionally `@ref` to pin a branch or tag.

## See what is installed

```bash
skd list            # one line per repository, plus the user's own skills
skd list --all      # every skill
skd list --agent claude
skd path <skill>    # directory of a skill's files
```

## Update

```bash
skd update          # all repositories; reports changed / added / upstream new / upstream gone
skd update owner/repo
```
After an update, summarise what changed for the user. Upstream changes are visible with `git -C <library> diff -- vendor/`. To undo an update: `git -C <library> checkout -- vendor skills.lock && skd apply`.

## Enable or disable per agent

```bash
skd enable <skill> --agent codex,cursor     # add agents
skd disable <skill> --agent codex           # remove from one agent
skd disable <skill>                         # unlink everywhere, keep in library
skd targets                                 # agents known on this machine
```

## Remove

- `skd disable <skill>` — unlink, keep files. Safe.
- `skd remove <skill>` — also drop a vendored skill from the library.
- `skd purge owner/repo` — delete a whole repository subscription. **Confirm with the user first.**

## Change a third-party skill

Never edit `vendor/`; the next update overwrites it. Instead:
```bash
skd fork <skill>            # copies it into skills/ as the user's own; upstream is deselected
```
Then edit the directory `skd path <skill>` prints.

## Create a new skill for the user

```bash
skd new <name>              # skills/<name>/SKILL.md, linked immediately
skd new <name> --private    # private/: this machine only, never pushed
```

## Back up / sync

```bash
skd sync                    # commit the library and push to its git remote
skd sync --remote <git-url> # set the remote first (ask the user for the URL)
```
Run `skd sync` after adding, updating or writing skills unless the user says otherwise. `private/` is a separate repository and is never pushed by `sync`.

## Rules

- `skd apply -n` previews; plain `skd apply` only touches links skd created. `skd apply --force` moves other people's directories into `<agent dir>/.skilldots-backup/<timestamp>/` — show the user the `-n` preview and get agreement before `--force`.
- Do not delete anything under an agent's skill directory yourself; use `skd disable`/`remove`/`purge`.
- If a skill the user wants is already present under a different name, say so instead of installing a duplicate.
- When `skd` is not installed, tell the user how: `go install github.com/Maybe404/skilldots/cmd/skd@latest` or the installer at https://github.com/Maybe404/skilldots.
