// Sample library for the browser preview. It mirrors the data in the approved
// design ("skilldots 主界面"), so screenshots can be compared side by side.
// Update history and trash entries are fictional and marked `example`.
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

export const HOME = "/Users/maybe"
export const LIBRARY = `${HOME}/code/github.com/Maybe404/agent-skills`

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

type Seed = [name: string, kind: Skill["kind"], origin: string, category: string, on: string, description: string]

// on: one digit per column agent (agents, claude, codex, gemini, grok, cursor)
const SEEDS: Seed[] = [
  ["skilldots", "vendor", "Maybe404/skilldots", "", "111111", "Manage the skills installed for AI agents on this machine."],
  ["ui-arsenal", "vendor", "Maybe404/ui-arsenal", "", "111000", "在前端 UI 任务里选择和获取成熟组件时使用。"],
  ["subagent", "skills", "", "", "111001", "派发 subagent 的规则：拆什么、派给谁、怎么验收。"],
  ["release-notes", "skills", "", "", "110000", "从 git log 生成发布说明，按用户可感知的改动分组。"],
  ["design-review", "skills", "", "", "010001", "按设计需求文档逐条审查界面截图。"],
  ["defuddle", "vendor", "kepano/obsidian-skills", "", "110000", "Extract clean Markdown from HTML pages with Defuddle."],
  ["json-canvas", "vendor", "kepano/obsidian-skills", "", "110000", "Create and edit JSON Canvas files (.canvas)."],
  ["obsidian-bases", "vendor", "kepano/obsidian-skills", "", "110000", "Create and edit Obsidian Bases (.base files)."],
  ["obsidian-cli", "vendor", "kepano/obsidian-skills", "", "111000", "Interact with Obsidian vaults using the Obsidian CLI."],
  ["obsidian-markdown", "vendor", "kepano/obsidian-skills", "", "110000", "Create and edit Obsidian Flavored Markdown."],
  ["impeccable", "vendor", "pbakaus/impeccable", "", "111101", "Use when the user wants to design, redesign, critique or polish a UI."],
  ["orchestration", "vendor", "stablyai/orca", "", "110000", "Coordinate supervised Orca workers."],
  ["orca-review", "vendor", "stablyai/orca", "", "010000", "Review hand-offs between Orca workers before merging."],
  ["pdf", "vendor", "anthropics/skills", "document-skills", "110100", "Read, create and edit PDF files."],
  ["docx", "vendor", "anthropics/skills", "document-skills", "010000", "Create and edit Word documents."],
  ["xlsx", "vendor", "anthropics/skills", "document-skills", "010000", "Create and analyze spreadsheets."],
  ["pptx", "vendor", "anthropics/skills", "document-skills", "011000", "Build and edit slide decks."],
  ["differential-review", "vendor", "trailofbits/skills", "differential-review", "011000", "Security-focused review of a diff."],
  ["modern-python", "vendor", "trailofbits/skills", "modern-python", "011000", "Project setup with uv, ruff and pytest."],
  ["find-skills", "vendor", "vercel-labs/agent-skills", "", "110001", "Search skills.sh and GitHub for a skill."],
  ["web-design-guidelines", "vendor", "vercel-labs/agent-skills", "", "110001", "Audit UI code against web design guidelines."],
  ["grill-me", "vendor", "mattpocock/skills", "engineering", "010000", "Interrogate a plan until every assumption is explicit."],
]

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
    description,
    dir: skillDir(kind, origin, category, name),
    targets: COLUMNS.filter((_, i) => on[i] === "1"),
  }))
}

export const CLAUDE_OVERRIDES = ["modern-python"]

export function initialPacks(): Pack[] {
  const p = (repo: string, commit: string, upstreamCount: number, license = "MIT", followAll = false): Pack => ({
    repo,
    commit,
    followAll,
    upstreamCount,
    license,
    url: `https://github.com/${repo}`,
  })
  return [
    p("Maybe404/skilldots", sha("4c1e9a2"), 1),
    p("Maybe404/ui-arsenal", sha("9b07d31"), 1),
    p("kepano/obsidian-skills", sha("e52f8c0"), 5, "MIT", true),
    p("pbakaus/impeccable", sha("a1d4e77"), 1, "Apache-2.0"),
    p("stablyai/orca", sha("0f3b9d4"), 3),
    p("anthropics/skills", sha("71c2b90"), 16, "Apache-2.0"),
    p("trailofbits/skills", sha("7c28e15"), 38, "CC-BY-SA-4.0"),
    p("vercel-labs/agent-skills", sha("2e8f1c4"), 12),
    p("mattpocock/skills", sha("5d90aa2"), 4),
  ]
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
        latest: sha("e52f8c0"),
        changed: ["skilldots"],
        commits: [
          { id: sha("e52f8c0"), title: "skilldots: 记录每次 apply 的改动摘要" },
          { id: sha("9a1c0d4"), title: "Fix trash restore path on case-insensitive volumes" },
        ],
      },
      {
        ...base,
        repo: "trailofbits/skills",
        commit: sha("7c28e15"),
        latest: sha("b3d0a61"),
        changed: ["differential-review"],
        moved: ["differential-review: differential-review → skills/differential-review"],
        commits: [
          { id: sha("b3d0a61"), title: "Move skills into skills/ subfolder" },
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

export function initialTrash(now: Date): TrashEntry[] {
  const day = 24 * 3600 * 1000
  return [
    {
      id: "20261017-213044/grill-me",
      name: "grill-me",
      origin: `${LIBRARY}/vendor/mattpocock/skills/grill-me`,
      path: `${LIBRARY}/.trash/20261017-213044/grill-me`,
      when: new Date(now.getTime() - 1 * day).toISOString(),
      reason: "removed",
      example: true,
    },
    {
      id: "20261012-090512/typography.md",
      name: "typography.md",
      origin: `${LIBRARY}/vendor/pbakaus/impeccable/impeccable/reference/typography.md`,
      path: `${LIBRARY}/.trash/20261012-090512/typography.md`,
      when: new Date(now.getTime() - 6 * day).toISOString(),
      reason: "update",
      example: true,
    },
    {
      id: "20261003-174420/pdf",
      name: "pdf",
      origin: `${HOME}/.claude/skills/pdf`,
      path: `${LIBRARY}/.trash/20261003-174420/pdf`,
      when: new Date(now.getTime() - 15 * day).toISOString(),
      reason: "adopt",
      example: true,
    },
  ]
}

/** Links that disagree with skills.toml (only with ?drift in the preview URL). */
export function initialPlan(): PlanItem[] {
  const drift = typeof location !== "undefined" && new URLSearchParams(location.search).has("drift")
  if (!drift) return []
  return [
    { action: "link", skill: "find-skills", agent: "gemini", path: `${HOME}/.gemini/skills/find-skills`, reason: "skills.toml changed on another machine" },
    { action: "unlink", skill: "orca-review", agent: "codex", path: `${HOME}/.codex/skills/orca-review`, reason: "no longer a target" },
    { action: "blocked", skill: "impeccable", agent: "grok", path: `${HOME}/.grok/skills/impeccable`, reason: "a directory skd did not create is in the way" },
  ]
}

export function initialScan(): ScanItem[] {
  return [
    {
      id: "s1",
      agent: "claude",
      name: "pdf",
      path: `${HOME}/.claude/skills/pdf`,
      state: "copy",
      source: "anthropics/skills",
      proposal: "pack",
      repo: "anthropics/skills",
      label: "subscribe to anthropics/skills",
    },
    {
      id: "s3",
      agent: "codex",
      name: "old-notes",
      path: `${HOME}/.codex/skills/old-notes`,
      state: "broken",
      linkTo: `${HOME}/Downloads/old-notes`,
      proposal: "drop",
      label: "broken link; remove",
    },
    {
      id: "s2",
      agent: "gemini",
      name: "release-notes",
      path: `${HOME}/.gemini/skills/release-notes`,
      state: "copy",
      proposal: "own",
      label: "copy into skills/ as your own (the library has one with the same name; you will be asked to rename)",
    },
    {
      id: "s4",
      agent: "cursor",
      name: "find-skills",
      path: `${HOME}/.cursor/skills/find-skills`,
      state: "copy",
      proposal: "enable",
      label: "enable for cursor (already in library)",
    },
  ]
}

const sk = (name: string, category: string, description: string): UpstreamSkill => ({ name, category, description })

/** Repositories the mock can "fetch". */
export const CATALOG: Record<string, Omit<RepoPreview, "source" | "subscribed" | "have">> = {
  "mattpocock/skills": {
    commit: sha("f01c9e3"),
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
    commit: sha("3ab72d0"),
    license: "Apache-2.0",
    rejected: [{ name: "../escape", dir: "skills/escape", reason: "name is not a safe directory name" }],
    skills: [
      sk("pdf", "document-skills", "Read, fill, merge and create PDF files."),
      sk("docx", "document-skills", "Create and edit Word documents with tracked changes."),
      sk("xlsx", "document-skills", "Work with spreadsheets: formulas, charts and cleaning data."),
      sk("pptx", "document-skills", "Create and edit PowerPoint decks."),
      sk("canvas-design", "creative", "Create posters and visual art as PNG or PDF."),
      sk("skill-creator", "meta", "Create and improve skills and measure how well they trigger."),
    ],
  },
  "trailofbits/skills": {
    commit: sha("b4410ea"),
    license: "CC-BY-SA-4.0",
    rejected: [],
    skills: [
      sk("differential-review", "differential-review", "Security-focused differential review of code changes."),
      sk("modern-python", "python", "Write Python the modern way: uv, ruff, type hints and pytest."),
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
    skills: [sk("ui-arsenal", "", "在前端 UI 任务里选择和获取成熟组件时使用。")],
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
    commit: sha("63555da"),
    license: "MIT",
    rejected: [],
    skills: [sk("skilldots", "", "Manage the skills installed for AI agents on this machine with the `skd` CLI.")],
  },
  "kepano/obsidian-skills": {
    commit: sha("e52f8c0"),
    license: "MIT",
    rejected: [],
    skills: [
      sk("defuddle", "", "Extract clean Markdown from HTML pages with Defuddle CLI."),
      sk("json-canvas", "", "Create and edit JSON Canvas files."),
      sk("obsidian-bases", "", "Create and edit Obsidian Bases."),
      sk("obsidian-cli", "", "Interact with Obsidian vaults using the Obsidian CLI."),
      sk("obsidian-markdown", "", "Obsidian Flavored Markdown."),
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
  extra(out, "private:html-document", {
    "scripts/build.ts": "import { readFile, writeFile } from \"node:fs/promises\"\n\nexport async function build(input: string, output: string): Promise<void> {\n  const md = await readFile(input, \"utf8\")\n  const html = `<!doctype html>\\n<main>${md}</main>`\n  await writeFile(output, html)\n}\n",
    "references/style.json": "{\n  \"font\": \"PingFang SC\",\n  \"maxWidth\": 720,\n  \"code\": { \"theme\": \"graphite\" }\n}\n",
  })
  extra(out, "private:idea-config-maintainer", {
    "scripts/validate.py": "\"\"\"Check a workflow export before importing it.\"\"\"\nimport sys\n\nimport yaml\n\n\ndef main(path: str) -> int:\n    with open(path, encoding=\"utf-8\") as f:\n        doc = yaml.safe_load(f)\n    nodes = doc[\"workflow\"][\"graph\"][\"nodes\"]\n    print(f\"{len(nodes)} nodes\")\n    return 0\n\n\nif __name__ == \"__main__\":\n    sys.exit(main(sys.argv[1]))\n",
    "references/nodes.yaml": "llm:\n  required: [model, prompt_template]\ncode:\n  required: [code, outputs]\nif-else:\n  required: [cases]\n",
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
    return "# 派发 subagent\n\n## 什么时候派\n\n- 任务能拆成**互不依赖**的几块\n- 需要读很多文件但只要结论\n\n## 怎么写指令\n\n1. 目标和验收标准\n2. 允许改哪些文件\n3. 回报格式\n\n```bash\nscripts/check.sh brief.md\n```\n\n| 情况 | 做法 |\n|---|---|\n| 结果不完整 | 打回，写明缺什么 |\n| 越界改动 | 回滚后重派 |\n"
  return `# ${name}\n\n## When to use\n\nSee the description above.\n\n## Steps\n\n1. Read the task.\n2. Do the work.\n3. Check the result.\n`
}
