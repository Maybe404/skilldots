// Sample library for the browser preview. It follows the data in the approved
// design ("skilldots 主界面"), so screenshots can be compared side by side, and
// it is kept self-consistent: commits, counts, descriptions, history, trash and
// the scan all describe the same library. Update history and trash entries are
// fictional and marked `example`. Nothing here is anyone's real machine.
import { SHARED_READERS } from "@/lib/agents"
import type {
  Agent,
  HistoryEntry,
  Outdated,
  Pack,
  PlanItem,
  RepoPreview,
  ScanItem,
  Skill,
  TrashEntry,
  UpstreamSkill,
} from "./types"

export const HOME = "/Users/you"
export const LIBRARY = `${HOME}/.skilldots`
/** Where the sample library is backed up (`skd sync`). */
export const REMOTE = "git@github.com:you/agent-skills.git"

/**
 * The real backend returns full 40-digit commit SHAs (`git rev-parse HEAD`,
 * `git log --format=%H`), and the interface must cope with that width. The
 * sample data writes the short form the design shows and expands it here to a
 * stable 40-digit id that starts with it.
 */
export function sha(short: string): string {
  let out = short
  let h = 0x811c9dc5
  for (let round = 0; out.length < 40; round++) {
    for (const c of `${short}:${round}`) h = Math.imul(h ^ c.charCodeAt(0), 0x01000193) >>> 0
    out += h.toString(16).padStart(8, "0")
  }
  return out.slice(0, 40)
}

// Registry order as the design shows it; the real registry has more agents,
// hidden by default when they are not on this machine.
const AGENT_DEFS: [string, string, string][] = [
  ["agents", "Agents", ".agents/skills"],
  ["claude", "Claude Code", ".claude/skills"],
  ["codex", "Codex", ".codex/skills"],
  ["gemini", "Gemini CLI", ".gemini/skills"],
  ["grok", "Grok", ".grok/skills"],
  ["cursor", "Cursor", ".cursor/skills"],
  ["opencode", "OpenCode", ".config/opencode/skills"],
  ["copilot", "GitHub Copilot", ".copilot/skills"],
  ["windsurf", "Windsurf", ".codeium/windsurf/skills"],
  ["kiro", "Kiro", ".kiro/skills"],
  ["qwen", "Qwen Code", ".qwen/skills"],
  ["trae", "Trae", ".trae/skills"],
  ["goose", "Goose", ".config/goose/skills"],
  ["roo", "Roo Code", ".roo/skills"],
  ["continue", "Continue", ".continue/skills"],
]
/**
 * Agents present on this machine, in registry order: the design's six, or as
 * many as `?agents=N` asks for in the preview URL (up to the whole registry).
 * A real machine often has 11 or more, which makes the Skill page's columns
 * and the top bar much tighter than the design's sample.
 */
function detectedAgents(): string[] {
  const asked = typeof location !== "undefined" ? Number(new URLSearchParams(location.search).get("agents")) : 0
  const n = Number.isInteger(asked) && asked > 6 ? Math.min(asked, AGENT_DEFS.length) : 6
  return AGENT_DEFS.slice(0, n).map(([name]) => name)
}

export function initialAgents(): Agent[] {
  const DETECTED = detectedAgents()
  return AGENT_DEFS.map(([name, label, rel]) => ({
    name,
    label,
    path: `${HOME}/${rel}`,
    detected: DETECTED.includes(name),
    // Like the real backend: every agent distributes unless paused; the ones
    // not on this machine are only hidden from the Skill page.
    enabled: true,
    hidden: !DETECTED.includes(name),
    isDefault: name === "agents" || name === "claude",
    builtin: true,
  }))
}

type Seed = [name: string, kind: Skill["kind"], origin: string, category: string, on: string, description?: string]

// on: one digit per column agent (agents, claude, codex, gemini, grok, cursor).
// Codex, Gemini CLI, Grok and Cursor read ~/.agents/skills too: a skill on
// Agents is never also linked into them (it would load twice), so a 1 there
// only appears for skills not on Agents.
// A vendored skill's description is upstream's, from CATALOG below.
const SEEDS: Seed[] = [
  ["skilldots", "vendor", "Maybe404/skilldots", "", "110000"],
  ["ui-arsenal", "vendor", "Maybe404/ui-arsenal", "", "110000"],
  ["subagent", "skills", "", "", "110000", "Brief a helper agent: the goal, the files it may touch and how the result is checked."],
  ["release-notes", "skills", "", "", "110000", "Draft release notes from git log, grouped by what users will notice."],
  ["design-review", "skills", "", "", "010001", "Review interface screenshots against the design spec, item by item."],
  ["defuddle", "vendor", "kepano/obsidian-skills", "", "110000"],
  ["json-canvas", "vendor", "kepano/obsidian-skills", "", "110000"],
  ["obsidian-bases", "vendor", "kepano/obsidian-skills", "", "110000"],
  ["obsidian-cli", "vendor", "kepano/obsidian-skills", "", "110000"],
  ["obsidian-markdown", "vendor", "kepano/obsidian-skills", "", "110000"],
  ["impeccable", "vendor", "pbakaus/impeccable", "", "110000"],
  ["orchestration", "vendor", "stablyai/orca", "", "110000"],
  ["orca-review", "vendor", "stablyai/orca", "", "010010"],
  ["pdf", "vendor", "anthropics/skills", "document-skills", "110000"],
  ["docx", "vendor", "anthropics/skills", "document-skills", "010000"],
  ["xlsx", "vendor", "anthropics/skills", "document-skills", "010000"],
  ["pptx", "vendor", "anthropics/skills", "document-skills", "011000"],
  ["differential-review", "vendor", "trailofbits/skills", "differential-review", "011000"],
  ["modern-python", "vendor", "trailofbits/skills", "modern-python", "011000"],
  ["find-skills", "vendor", "vercel-labs/agent-skills", "", "110000"],
  ["web-design-guidelines", "vendor", "vercel-labs/agent-skills", "", "110000"],
  ["grill-me", "vendor", "mattpocock/skills", "engineering", "010100"],
]

/** Upstream's description of a vendored skill (as CATALOG has it). */
export function upstreamDescription(repo: string, name: string): string {
  return CATALOG[repo]?.skills.find((s) => s.name === name)?.description ?? ""
}

export function skillDir(kind: Skill["kind"], origin: string, category: string, name: string): string {
  switch (kind) {
    case "vendor":
      return `${LIBRARY}/vendor/${origin}/${category ? category + "/" : ""}${name}`
    case "skills":
      return `${LIBRARY}/skills/${name}`
    case "private":
      return `${LIBRARY}/private/${name}`
    case "app":
      return `/Applications/Surge.app/Contents/Resources/Skills/${name}`
  }
}

export function refKey(kind: Skill["kind"], origin: string, name: string): string {
  switch (kind) {
    case "vendor":
      return `${origin}:${name}`
    case "app":
      return `app:${name}`
    default:
      return `${kind}:${name}`
  }
}

export function initialSkills(): Skill[] {
  const COLUMNS = detectedAgents()
  return SEEDS.map(([name, kind, origin, category, on, description]) => ({
    ref: { key: refKey(kind, origin, name) },
    name,
    linkName: name,
    kind,
    origin,
    category,
    description: description ?? upstreamDescription(origin, name),
    dir: skillDir(kind, origin, category, name),
    targets: COLUMNS.filter((_, i) => on[i] === "1"),
  }))
}

export const CLAUDE_OVERRIDES = ["modern-python"]

/** The commit each subscribed repository is vendored at (skills.lock). */
const VENDORED: [repo: string, short: string, license: string, followAll?: boolean][] = [
  ["Maybe404/skilldots", "4c1e9a2", "MIT"],
  ["Maybe404/ui-arsenal", "9b07d31", "MIT"],
  ["kepano/obsidian-skills", "e52f8c0", "MIT", true],
  ["pbakaus/impeccable", "a1d4e77", "Apache-2.0"],
  ["stablyai/orca", "0f3b9d4", "MIT"],
  ["anthropics/skills", "71c2b90", "Apache-2.0"],
  ["trailofbits/skills", "7c28e15", "CC-BY-SA-4.0"],
  ["vercel-labs/agent-skills", "2e8f1c4", "MIT"],
  ["mattpocock/skills", "5d90aa2", "MIT"],
]

export function initialPacks(): Pack[] {
  return VENDORED.map(([repo, short, license, followAll = false]) => ({
    repo,
    commit: sha(short),
    followAll,
    // None of the sample's pending updates adds or drops a skill, so the
    // count at the last update is what upstream has now.
    upstreamCount: CATALOG[repo]?.skills.length ?? 1,
    license,
    url: `https://github.com/${repo}`,
  }))
}

export function initialOutdated(now: Date): Outdated {
  const checked = new Date(now.getTime() - 20 * 1000).toISOString()
  const base = { changed: [], moved: [], new: [], gone: [], pending: [], commits: [], checked }
  return {
    checked,
    failed: [],
    packs: [
      {
        ...base,
        repo: "Maybe404/skilldots",
        commit: sha("4c1e9a2"),
        latest: sha("8d3f6b1"),
        changed: ["skilldots"],
        commits: [
          { id: sha("8d3f6b1"), title: "skilldots: say what apply changed in one line" },
          { id: sha("9a1c0d4"), title: "Fix trash restore path on case-insensitive volumes" },
        ],
      },
      {
        ...base,
        repo: "trailofbits/skills",
        commit: sha("7c28e15"),
        latest: sha("b3d0a61"),
        changed: ["differential-review"],
        moved: ["differential-review: differential-review → security"],
        commits: [
          { id: sha("b3d0a61"), title: "Group the review plugins under plugins/security" },
          { id: sha("2e41f7c"), title: "differential-review: add CWE hints" },
        ],
      },
      ...initialPacks()
        .filter((p) => p.repo !== "Maybe404/skilldots" && p.repo !== "trailofbits/skills")
        .map((p) => ({ ...base, repo: p.repo, commit: p.commit, latest: p.commit })),
    ],
  }
}

/** A past day at 11:30 local time. */
function daysAgoAt(now: Date, days: number): string {
  const d = new Date(now)
  d.setDate(d.getDate() - days)
  d.setHours(11, 30, 0, 0)
  return d.toISOString()
}

export function initialHistory(now: Date): HistoryEntry[] {
  return [
    {
      id: "h3",
      when: daysAgoAt(now, 1),
      repo: "kepano/obsidian-skills",
      from: sha("c7a0912"),
      to: sha("e52f8c0"),
      changed: ["obsidian-cli", "obsidian-markdown"],
      added: [],
      gone: [],
      moved: [],
      commits: [
        { id: sha("e52f8c0"), title: "obsidian-cli: document the search command" },
        { id: sha("d1b7a35"), title: "Clarify callout syntax for nested blocks" },
      ],
      backups: [],
      example: true,
    },
    {
      id: "h2",
      when: daysAgoAt(now, 6),
      repo: "pbakaus/impeccable",
      from: sha("5e0c2b8"),
      to: sha("a1d4e77"),
      changed: ["impeccable"],
      added: [],
      gone: [],
      moved: [],
      commits: [{ id: sha("a1d4e77"), title: "Add the harden command for edge cases" }],
      backups: ["impeccable/reference/typography.md"],
      example: true,
    },
    {
      id: "h1",
      when: daysAgoAt(now, 15),
      repo: "vercel-labs/agent-skills",
      from: sha("88d01aa"),
      to: sha("2e8f1c4"),
      changed: [],
      added: ["find-skills"],
      gone: [],
      moved: [],
      commits: [{ id: sha("2e8f1c4"), title: "find-skills: search skills.sh as well" }],
      backups: [],
      example: true,
    },
  ]
}

/** The batch directory name skd gives what it moves to the trash at `iso` (UTC). */
export function trashBatch(iso: string): string {
  return iso.replace(/[-:]/g, "").replace("T", "-").slice(0, 15)
}

export function initialTrash(now: Date): TrashEntry[] {
  const entry = (name: string, origin: string, when: string, reason: TrashEntry["reason"]): TrashEntry => ({
    id: `${trashBatch(when)}/${name}`,
    name,
    origin,
    path: `${LIBRARY}/.trash/${trashBatch(when)}/${name}`,
    when,
    reason,
    example: true,
  })
  const ago = (days: number, minutes: number) => new Date(now.getTime() - (days * 24 * 60 + minutes) * 60 * 1000).toISOString()
  return [
    // Removed yesterday; the pack keeps its other skill, grill-me.
    entry("write-a-prd", `${LIBRARY}/vendor/mattpocock/skills/planning/write-a-prd`, ago(1, 40), "removed"),
    // Your edit, set aside by the impeccable update (history h2).
    entry("typography.md", `${LIBRARY}/vendor/pbakaus/impeccable/impeccable/reference/typography.md`, daysAgoAt(now, 6), "update"),
    // The copy in Claude Code's folder, replaced by a link when pdf was adopted.
    entry("pdf", `${HOME}/.claude/skills/pdf`, ago(15, 90), "adopt"),
  ]
}

/** Links that disagree with skills.toml (only with ?drift in the preview URL). */
export function initialPlan(): PlanItem[] {
  const drift = typeof location !== "undefined" && new URLSearchParams(location.search).has("drift")
  if (!drift) return []
  return [
    { action: "link", skill: "docx", agent: "claude", path: `${HOME}/.claude/skills/docx`, reason: "skills.toml changed on another machine" },
    { action: "unlink", skill: "orca-review", agent: "codex", path: `${HOME}/.codex/skills/orca-review`, reason: "no longer a target" },
    { action: "blocked", skill: "impeccable", agent: "claude", path: `${HOME}/.claude/skills/impeccable`, reason: "a directory skd did not create is in the way" },
  ]
}

/** What the scan finds in the agent folders that skilldots does not manage. */
const FOUND: Pick<ScanItem, "id" | "agent" | "name" | "state" | "source" | "linkTo">[] = [
  // Installed with `npx skills add`, so its upstream is known.
  { id: "s1", agent: "claude", name: "canvas-design", state: "copy", source: "anthropics/skills" },
  { id: "s2", agent: "codex", name: "find-skills", state: "copy", source: "vercel-labs/agent-skills" },
  { id: "s3", agent: "codex", name: "old-notes", state: "broken", linkTo: `${HOME}/Downloads/old-notes` },
  { id: "s4", agent: "gemini", name: "release-notes", state: "copy" },
]

/**
 * The scan of FOUND against a library, with the proposal skd would make
 * (app.Proposals): a name the library has becomes "enable" (or nothing, when
 * that agent already gets it), a broken link is dropped, a copy with a known
 * upstream subscribes to it, anything else is copied in as your own.
 */
export function initialScan(library: Skill[]): ScanItem[] {
  const out: ScanItem[] = []
  for (const f of FOUND) {
    const path = `${HOME}/${AGENT_DEFS.find(([n]) => n === f.agent)![2]}/${f.name}`
    const have = library.find((s) => s.linkName.toLowerCase() === f.name.toLowerCase())
    if (have?.targets.includes(f.agent)) continue
    if (have) {
      // An agent that reads ~/.agents/skills already gets a library skill on Agents from there: its own copy is a
      // second one, which adopting moves to the trash (app.Proposals); otherwise the library's skill is enabled for it.
      const second = have.targets.includes("agents") && SHARED_READERS.includes(f.agent)
      const label = second ? `${f.agent} already reads the library's copy from ~/.agents/skills; this second copy goes to the trash` : `enable for ${f.agent} (already in library)`
      out.push({ ...f, path, proposal: "enable", sharedCopy: second, label })
    } else if (f.state === "broken") out.push({ ...f, path, proposal: "drop", label: "broken link; move to the trash" })
    else if (f.source) out.push({ ...f, path, proposal: "pack", repo: f.source, label: `subscribe to ${f.source}` })
    else out.push({ ...f, path, proposal: "own", as: f.name, label: "copy into skills/ as your own (no known upstream)" })
  }
  return out
}

const sk = (name: string, category: string, description: string): UpstreamSkill => ({ name, category, description })

/**
 * Repositories the mock can "fetch": what upstream has now. A subscribed
 * repository's commit is the vendored one unless initialOutdated lists an
 * update for it, and the descriptions are the vendored skills' descriptions.
 */
export const CATALOG: Record<string, Omit<RepoPreview, "source" | "subscribed" | "have">> = {
  "mattpocock/skills": {
    commit: sha("5d90aa2"),
    license: "MIT",
    rejected: [],
    skills: [
      sk("tdd", "engineering", "Test-driven development loop: write the failing test, make it pass, refactor."),
      sk("code-review", "engineering", "Review a diff for correctness, naming and missing tests before merging."),
      sk("grill-me", "engineering", "Interview the user hard about a plan until every assumption is explicit."),
      sk("write-a-prd", "planning", "Turn a rough idea into a product requirements document with user stories."),
      sk("prd-to-issues", "planning", "Split a PRD into independently shippable GitHub issues."),
      sk("edit-article", "writing", "Tighten an article: structure, clarity and cutting what does not earn its place."),
    ],
  },
  "anthropics/skills": {
    commit: sha("71c2b90"),
    license: "Apache-2.0",
    rejected: [{ name: "../escape", dir: "skills/escape", reason: "name is not a safe directory name" }],
    skills: [
      sk("pdf", "document-skills", "Read, fill, merge and create PDF files."),
      sk("docx", "document-skills", "Create and edit Word documents with tracked changes."),
      sk("xlsx", "document-skills", "Work with spreadsheets: formulas, charts and cleaning data."),
      sk("pptx", "document-skills", "Create and edit PowerPoint decks."),
      sk("doc-coauthoring", "document-skills", "Write a document together with the user, section by section."),
      sk("algorithmic-art", "creative", "Generative art with p5.js, seeded and reproducible."),
      sk("canvas-design", "creative", "Create posters and visual art as PNG or PDF."),
      sk("slack-gif-creator", "creative", "Make small animated GIFs that fit Slack's limits."),
      sk("theme-factory", "creative", "Apply a ready-made colour and font theme to slides, documents or pages."),
      sk("brand-guidelines", "enterprise", "Apply a brand's colours and typography to anything you make."),
      sk("internal-comms", "enterprise", "Write status reports, newsletters and other internal updates."),
      sk("frontend-design", "development", "Build distinctive, production-grade web interfaces."),
      sk("mcp-builder", "development", "Build an MCP server that gives an agent a well-designed set of tools."),
      sk("web-artifacts-builder", "development", "Build multi-component HTML artifacts with React and Tailwind."),
      sk("webapp-testing", "development", "Test a local web app with Playwright: screenshots, logs and clicks."),
      sk("skill-creator", "meta", "Create and improve skills and measure how well they trigger."),
    ],
  },
  "trailofbits/skills": {
    commit: sha("b3d0a61"),
    license: "CC-BY-SA-4.0",
    rejected: [],
    skills: [
      sk("differential-review", "security", "Security-focused differential review of code changes."),
      sk("modern-python", "modern-python", "Write Python the modern way: uv, ruff, type hints and pytest."),
      sk("semgrep-rule-creator", "static-analysis", "Write and test Semgrep rules for a bug class."),
      sk("codeql", "static-analysis", "Run CodeQL queries and triage the results."),
      sk("constant-time-analysis", "crypto", "Check cryptographic code for timing side channels."),
      sk("audit-context-building", "audit", "Build a mental model of an unfamiliar codebase before auditing it."),
    ],
  },
  "vercel-labs/agent-skills": {
    commit: sha("2e8f1c4"),
    license: "MIT",
    rejected: [],
    skills: [
      sk("find-skills", "", "Search skills.sh and GitHub for a skill."),
      sk("web-design-guidelines", "", "Audit UI code against web design guidelines."),
      sk("vercel-react-best-practices", "", "React and Next.js performance optimization guidelines from Vercel Engineering."),
      sk("vercel-deploy", "", "Deploy the current project to Vercel."),
      sk("composition-patterns", "", "React composition patterns that scale."),
    ],
  },
  "Maybe404/ui-arsenal": {
    commit: sha("9b07d31"),
    license: "MIT",
    rejected: [],
    skills: [sk("ui-arsenal", "", "Pick mature UI components, motion and icons from a curated catalogue for front-end work.")],
  },
  "pbakaus/impeccable": {
    commit: sha("a1d4e77"),
    license: "Apache-2.0",
    rejected: [],
    skills: [sk("impeccable", "", "Use when the user wants to design, redesign, critique or polish a UI.")],
  },
  "stablyai/orca": {
    commit: sha("0f3b9d4"),
    license: "MIT",
    rejected: [],
    skills: [
      sk("orchestration", "", "Coordinate supervised Orca workers."),
      sk("orca-review", "", "Review hand-offs between Orca workers before merging."),
      sk("orca-cli", "", "Drive Orca worktrees and terminals from the command line."),
    ],
  },
  "Maybe404/skilldots": {
    commit: sha("8d3f6b1"),
    license: "MIT",
    rejected: [],
    skills: [sk("skilldots", "", "Manage the skills installed for AI agents on this machine with the skd CLI.")],
  },
  "kepano/obsidian-skills": {
    commit: sha("e52f8c0"),
    license: "MIT",
    rejected: [],
    skills: [
      sk("defuddle", "", "Extract clean Markdown from HTML pages with Defuddle CLI."),
      sk("json-canvas", "", "Create and edit JSON Canvas files (.canvas)."),
      sk("obsidian-bases", "", "Create and edit Obsidian Bases (.base files)."),
      sk("obsidian-cli", "", "Interact with Obsidian vaults using the Obsidian CLI."),
      sk("obsidian-markdown", "", "Create and edit Obsidian Flavored Markdown."),
    ],
  },
}

/** Files of each skill for the in-app editor, keyed by ref key. */
export function initialFiles(skills: Skill[]): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {}
  for (const s of skills) {
    out[s.ref.key] = { "SKILL.md": skillMd(s.name, s.description, bodyFor(s.name)) }
  }
  extra(out, "skills:subagent", {
    "references/briefing.md": "# Writing a brief\n\n- State the goal and what *done* looks like.\n- List the files the agent may touch.\n- Say how the result will be checked.\n",
    "scripts/check.sh": "#!/usr/bin/env bash\nset -euo pipefail\n\n# Fail when a brief is missing its acceptance section.\nfor f in \"$@\"; do\n  grep -q '^## Acceptance' \"$f\" || { echo \"missing acceptance: $f\"; exit 1; }\ndone\n",
  })
  extra(out, "Maybe404/ui-arsenal:ui-arsenal", {
    "scripts/find.sh": "#!/usr/bin/env bash\n# Search the local catalogue.\nset -euo pipefail\ngrep -i -- \"$1\" sources/*.tsv | head -20\n",
    "guides/table.md": "# Tables\n\nUse `shadcn:data-table` with TanStack Table. Add `aria-sort` yourself.\n",
  })
  extra(out, "kepano/obsidian-skills:obsidian-cli", {
    "references/commands.md": "| Command | Does |\n|---|---|\n| `obsidian search` | full-text search |\n| `obsidian open` | open a note |\n",
  })
  extra(out, "app:surge", {
    "scripts/status.sh": "#!/bin/sh\nsurge-cli --raw dump active | head\n",
  })
  return out
}

function extra(out: Record<string, Record<string, string>>, key: string, files: Record<string, string>) {
  if (out[key]) Object.assign(out[key], files)
}

function skillMd(name: string, description: string, body: string): string {
  const desc = description.includes(":") || description.includes("\"") ? JSON.stringify(description) : description
  return `---\nname: ${name}\ndescription: ${desc}\n---\n\n${body}`
}

function bodyFor(name: string): string {
  if (name === "test") return "# test\n\nTODO: describe what this skill does.\n"
  if (name === "subagent")
    return "# Briefing a subagent\n\n## When to hand work off\n\n- The task splits into pieces that **do not depend** on each other\n- Many files need reading, but only the conclusion matters\n\n## What a brief says\n\n1. The goal and how it will be checked\n2. The files it may change\n3. What to report back\n\n```bash\nscripts/check.sh brief.md\n```\n\n| Result | Next step |\n|---|---|\n| Incomplete | Send it back, saying what is missing |\n| Changed files outside its scope | Revert them and brief it again |\n"
  return `# ${name}\n\n## When to use\n\nSee the description above.\n\n## Steps\n\n1. Read the task.\n2. Do the work.\n3. Check the result.\n`
}
