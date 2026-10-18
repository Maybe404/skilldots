import type { Agent } from "@/lib/backend"

// The Go bridge reports the registry name as the label ("claude"); these are
// the names people know the tools by.
const LABELS: Record<string, string> = {
  agents: "Agents",
  claude: "Claude Code",
  codex: "Codex",
  gemini: "Gemini CLI",
  grok: "Grok",
  cursor: "Cursor",
  opencode: "OpenCode",
  copilot: "GitHub Copilot",
  windsurf: "Windsurf",
  kiro: "Kiro",
  qwen: "Qwen Code",
  trae: "Trae",
  goose: "Goose",
  antigravity: "Antigravity",
  roo: "Roo Code",
  kilocode: "Kilo Code",
  continue: "Continue",
  augment: "Augment",
  junie: "Junie",
  iflow: "iFlow",
  qoder: "Qoder",
  codebuddy: "CodeBuddy",
  openhands: "OpenHands",
  crush: "Crush",
  factory: "Factory",
  pi: "Pi",
}

// Two-letter monograms, as in the design ("Ag", "CC", "Cx" …).
const MONOS: Record<string, string> = {
  agents: "Ag",
  claude: "CC",
  codex: "Cx",
  gemini: "Ge",
  grok: "Gk",
  cursor: "Cu",
  opencode: "Oc",
  copilot: "Co",
  windsurf: "Ws",
  kiro: "Ki",
  qwen: "Qw",
  trae: "Tr",
  goose: "Go",
  antigravity: "Av",
  roo: "Ro",
  kilocode: "Kc",
  continue: "Ct",
  augment: "Au",
  junie: "Ju",
  iflow: "iF",
  qoder: "Qo",
  codebuddy: "Cb",
  openhands: "Oh",
  crush: "Cr",
  factory: "Fa",
  pi: "Pi",
}

export function agentLabel(a: Pick<Agent, "name" | "label">): string {
  if (a.label && a.label !== a.name) return a.label
  return LABELS[a.name] ?? a.name
}

export function agentMono(a: Pick<Agent, "name">): string {
  const known = MONOS[a.name]
  if (known) return known
  const n = a.name.replace(/[^A-Za-z0-9]/g, "")
  return (n.slice(0, 1).toUpperCase() + n.slice(1, 2)) || "?"
}

/** "/Users/maybe/.claude/skills" → "~/.claude/skills" */
export function tildify(path: string): string {
  return path.replace(/^\/(?:Users|home)\/[^/]+(?=\/|$)/, "~")
}
