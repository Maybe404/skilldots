// Sample library modelled on a real one. Update history and trash entries are
// fictional and marked `example`.
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
const ENABLED = ["agents", "claude", "codex", "gemini", "grok"]
const DETECTED = [...ENABLED, "cursor", "opencode"]

export function initialAgents(): Agent[] {
  return AGENT_DEFS.map(([name, label, rel]) => ({
    name,
    label,
    path: `~/${rel}`,
    detected: DETECTED.includes(name),
    enabled: ENABLED.includes(name),
    isDefault: name === "agents" || name === "claude",
    builtin: true,
  }))
}

type Seed = [name: string, kind: Skill["kind"], origin: string, category: string, targets: string[], description: string]

const SEEDS: Seed[] = [
  ["skilldots", "vendor", "Maybe404/skilldots", "", ["agents", "claude", "codex", "gemini", "grok"],
    "Manage the skills installed for AI agents on this machine with the `skd` CLI (skilldots). Use when the user asks to install, add, find, update, remove, enable, disable, fork, list, back up or sync agent skills, or mentions skills.toml, a skill library, or a GitHub repository of skills."],
  ["ui-arsenal", "vendor", "Maybe404/ui-arsenal", "", ["agents", "claude", "codex"],
    "在前端 UI 任务里选择和获取成熟组件时使用：新增或替换组件、调整页面视觉、加交互或动效、背景、加载态、图标、AI 界面、落地页或后台页面。"],
  ["defuddle", "vendor", "kepano/obsidian-skills", "", ["agents", "claude"],
    "Extract clean Markdown from HTML pages with Defuddle CLI."],
  ["json-canvas", "vendor", "kepano/obsidian-skills", "", ["agents", "claude"],
    "Create and edit JSON Canvas files (.canvas) with nodes, edges, groups, and connections. Use when working with .canvas files or Obsidian canvases."],
  ["obsidian-bases", "vendor", "kepano/obsidian-skills", "", ["agents", "claude"],
    "Create and edit Obsidian Bases (.base files) with views, filters, formulas, and summaries."],
  ["obsidian-cli", "vendor", "kepano/obsidian-skills", "", ["agents", "claude", "codex"],
    "Interact with Obsidian vaults using the Obsidian CLI to read, create, search, and manage notes, tasks and properties."],
  ["obsidian-markdown", "vendor", "kepano/obsidian-skills", "", ["agents", "claude"],
    "Create and edit Obsidian Flavored Markdown with wikilinks, embeds, callouts, properties, and other Obsidian-specific syntax."],
  ["impeccable", "vendor", "pbakaus/impeccable", "", ["agents", "claude", "codex", "gemini"],
    "Use when the user wants to design, redesign, critique, audit, polish or simplify a frontend interface: websites, dashboards, product UI, forms, onboarding and empty states."],
  ["orchestration", "vendor", "stablyai/orca", "", ["agents", "claude"],
    "Coordinate supervised Orca workers: threaded messages, blocking ask/reply, task dispatch, task DAGs, decision gates and coordinator loops."],
  ["differential-review", "vendor", "trailofbits/skills", "differential-review", ["agents", "claude", "codex"],
    "Performs security-focused differential review of code changes. Use when reviewing a PR, commit, or diff for security vulnerabilities."],
  ["modern-python", "vendor", "trailofbits/skills", "modern-python", ["agents", "claude"],
    "Write Python the modern way: uv, ruff, type hints, pyproject.toml and pytest. Use when creating or upgrading a Python project."],
  ["vercel-react-best-practices", "vendor", "vercel-labs/agent-skills", "", ["agents", "claude"],
    "React and Next.js performance optimization guidelines from Vercel Engineering. Use when writing, reviewing or refactoring React code."],
  ["find-skills", "vendor", "vercel-labs/skills", "", ["agents", "claude", "codex", "gemini", "grok"],
    "Helps users discover and install agent skills when they ask \"how do I do X\" or \"is there a skill that can…\"."],
  ["subagent", "skills", "", "", ["agents", "claude", "codex"],
    "派发 subagent 的规则：拆什么、派给谁、怎么写指令、怎么验收和打回。派发任何后台 agent 之前使用。"],
  ["curated-ui", "skills", "", "", ["agents", "claude"],
    "Prefer mature, production-oriented UI components, interaction patterns, effects and icons from the user's curated sources when building or reviewing frontend interfaces."],
  ["test", "skills", "", "", ["agents", "codex"],
    "What this skill does and when an agent should use it."],
  ["html-document", "private", "", "", ["agents", "claude"],
    "将 markdown 转 HTML，生成美观的可交互单页和内部知识库文档。"],
  ["idea-config-maintainer", "private", "", "", ["agents"],
    "idea/dify 二开低代码平台 yaml 工作流配置的编写、检查、维护、重构和优化。"],
  ["surge", "app", "surge", "", ["agents", "claude"],
    "Control and troubleshoot the Surge proxy app through surge-cli. Use whenever a task involves Surge in any way."],
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
  return SEEDS.map(([name, kind, origin, category, targets, description]) => ({
    ref: { key: refKey(kind, origin, name) },
    name,
    linkName: name,
    kind,
    origin,
    category,
    description,
    dir: skillDir(kind, origin, category, name),
    targets: [...targets],
  }))
}

export const CLAUDE_OVERRIDES = ["modern-python", "vercel-react-best-practices"]

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
    p("Maybe404/skilldots", "4c1e9a2", 1),
    p("Maybe404/ui-arsenal", "9b07d31", 1),
    p("kepano/obsidian-skills", "e52f8c0", 5, "MIT", true),
    p("pbakaus/impeccable", "a1d4e77", 1, "Apache-2.0"),
    p("stablyai/orca", "0f3b9d4", 3),
    p("trailofbits/skills", "7c28e15", 38, "CC-BY-SA-4.0"),
    p("vercel-labs/agent-skills", "d93a6b0", 6),
    p("vercel-labs/skills", "2e8f1c4", 2),
  ]
}

export function initialOutdated(now: Date): Outdated {
  const checked = new Date(now.getTime() - 2 * 60 * 1000).toISOString()
  const base = { changed: [], moved: [], new: [], gone: [], pending: [], commits: [], checked }
  return {
    checked,
    failed: [],
    packs: [
      {
        ...base,
        repo: "Maybe404/skilldots",
        commit: "4c1e9a2",
        latest: "63555da",
        changed: ["skilldots"],
        commits: [
          { id: "63555da", title: "test(tui): cover the dashboard's hangup handling" },
          { id: "1ecb606", title: "fix(cache): clear the lock file git names, not a fixed list" },
          { id: "6a140e1", title: "docs: say that updating a pack with only moved skills renames them" },
          { id: "1da0bdd", title: "fix(tui): run the background check without a terminal and stop it on hangup" },
        ],
      },
      {
        ...base,
        repo: "trailofbits/skills",
        commit: "7c28e15",
        latest: "b4410ea",
        moved: ["modern-python: modern-python → python/modern-python"],
        commits: [{ id: "b4410ea", title: "Group Python skills under plugins/python" }],
      },
      ...["Maybe404/ui-arsenal", "kepano/obsidian-skills", "pbakaus/impeccable", "stablyai/orca", "vercel-labs/agent-skills", "vercel-labs/skills"].map(
        (repo) => {
          const c = initialPacks().find((p) => p.repo === repo)!.commit
          return { ...base, repo, commit: c, latest: c }
        },
      ),
    ],
  }
}

export function initialHistory(now: Date): HistoryEntry[] {
  const day = 24 * 3600 * 1000
  return [
    {
      id: "h3",
      when: new Date(now.getTime() - 1 * day).toISOString(),
      repo: "kepano/obsidian-skills",
      from: "c7a0912",
      to: "e52f8c0",
      changed: ["obsidian-cli", "obsidian-markdown"],
      added: [],
      gone: [],
      moved: [],
      commits: [
        { id: "e52f8c0", title: "obsidian-cli: document the search command" },
        { id: "d1b7a35", title: "Clarify callout syntax for nested blocks" },
      ],
      backups: [],
      example: true,
    },
    {
      id: "h2",
      when: new Date(now.getTime() - 6 * day).toISOString(),
      repo: "pbakaus/impeccable",
      from: "5e0c2b8",
      to: "a1d4e77",
      changed: ["impeccable"],
      added: [],
      gone: [],
      moved: [],
      commits: [{ id: "a1d4e77", title: "Add the harden command for edge cases" }],
      backups: ["impeccable/reference/typography.md"],
      example: true,
    },
    {
      id: "h1",
      when: new Date(now.getTime() - 15 * day).toISOString(),
      repo: "vercel-labs/skills",
      from: "88d01aa",
      to: "2e8f1c4",
      changed: [],
      added: ["find-skills"],
      gone: [],
      moved: [],
      commits: [{ id: "2e8f1c4", title: "find-skills: search skills.sh as well" }],
      backups: [],
      example: true,
    },
  ]
}

export function initialTrash(now: Date): TrashEntry[] {
  const day = 24 * 3600 * 1000
  return [
    {
      id: "20261016-213044/grill-me",
      name: "grill-me",
      origin: `${LIBRARY}/vendor/mattpocock/skills/grill-me`,
      path: `${LIBRARY}/.trash/20261016-213044/grill-me`,
      when: new Date(now.getTime() - 1 * day).toISOString(),
      example: true,
    },
    {
      id: "20261011-090512/impeccable",
      name: "impeccable",
      origin: `${LIBRARY}/vendor/pbakaus/impeccable/impeccable`,
      path: `${LIBRARY}/.trash/20261011-090512/impeccable`,
      when: new Date(now.getTime() - 6 * day).toISOString(),
      example: true,
    },
    {
      id: "20261002-174420/pdf",
      name: "pdf",
      origin: `${HOME}/.claude/skills/pdf`,
      path: `${LIBRARY}/.trash/20261002-174420/pdf`,
      when: new Date(now.getTime() - 15 * day).toISOString(),
      example: true,
    },
  ]
}

export function initialPlan(): PlanItem[] {
  return [
    { action: "link", skill: "find-skills", agent: "gemini", path: "~/.gemini/skills/find-skills", reason: "skills.toml changed on another machine" },
    { action: "unlink", skill: "test", agent: "codex", path: "~/.codex/skills/test", reason: "no longer a target" },
    { action: "blocked", skill: "impeccable", agent: "grok", path: "~/.grok/skills/impeccable", reason: "a directory skd did not create is in the way" },
  ]
}

export function initialScan(): ScanItem[] {
  return [
    {
      id: "s1",
      agent: "claude",
      name: "pdf",
      path: "~/.claude/skills/pdf",
      state: "copy",
      source: "anthropics/skills",
      proposal: "pack",
      repo: "anthropics/skills",
      label: "subscribe to anthropics/skills",
    },
    {
      id: "s2",
      agent: "gemini",
      name: "release-notes",
      path: "~/.gemini/skills/release-notes",
      state: "copy",
      proposal: "own",
      label: "copy into skills/ as your own (no known upstream)",
    },
    {
      id: "s3",
      agent: "codex",
      name: "old-notes",
      path: "~/.codex/skills/old-notes",
      state: "broken",
      linkTo: "~/Downloads/old-notes",
      proposal: "drop",
      label: "broken link; remove",
    },
    {
      id: "s4",
      agent: "cursor",
      name: "find-skills",
      path: "~/.cursor/skills/find-skills",
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
    commit: "f01c9e3",
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
    commit: "3ab72d0",
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
    commit: "b4410ea",
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
    commit: "d93a6b0",
    license: "MIT",
    rejected: [],
    skills: [
      sk("vercel-react-best-practices", "", "React and Next.js performance optimization guidelines from Vercel Engineering."),
      sk("web-design-guidelines", "", "Review UI code against web interface guidelines."),
      sk("vercel-deploy", "", "Deploy the current project to Vercel."),
      sk("composition-patterns", "", "React composition patterns that scale."),
    ],
  },
  "Maybe404/skilldots": {
    commit: "63555da",
    license: "MIT",
    rejected: [],
    skills: [sk("skilldots", "", "Manage the skills installed for AI agents on this machine with the `skd` CLI.")],
  },
  "kepano/obsidian-skills": {
    commit: "e52f8c0",
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
