import type { Skill } from "@/lib/backend"

/*
 * Link changes the user has made but not applied yet (the design's "待应用").
 * The backend saves and applies every enable/disable/setTargets call at once,
 * so the desktop collects the wanted state here and calls setTargets per skill
 * when the user presses 应用.
 *
 * Keyed by skill ref key and agent name; the value is the wanted state.
 */
export type Pending = Record<string, boolean>

const SEP = "\u0000"
export const pendingKey = (skill: string, agent: string) => skill + SEP + agent
export const splitKey = (k: string) => k.split(SEP) as [skill: string, agent: string]

export type CellState = "on" | "off" | "adding" | "removing"

export function cellState(skill: Skill, agent: string, pending: Pending): CellState {
  const base = skill.targets.includes(agent)
  const k = pendingKey(skill.ref.key, agent)
  if (!(k in pending)) return base ? "on" : "off"
  return pending[k] ? "adding" : "removing"
}

/** Installed once the pending changes are applied. */
export function isOn(skill: Skill, agent: string, pending: Pending): boolean {
  const s = cellState(skill, agent, pending)
  return s === "on" || s === "adding"
}

/** Flip one cell: a pending change is reverted, otherwise one is recorded. */
export function toggle(pending: Pending, skill: Skill, agent: string): Pending {
  const k = pendingKey(skill.ref.key, agent)
  const next = { ...pending }
  if (k in next) delete next[k]
  else next[k] = !skill.targets.includes(agent)
  return next
}

/** Drops entries for skills that are gone or already in the wanted state. */
export function prune(pending: Pending, skills: Skill[]): Pending {
  const byKey = new Map(skills.map((s) => [s.ref.key, s]))
  const next: Pending = {}
  let changed = false
  for (const [k, want] of Object.entries(pending)) {
    const [key, agent] = splitKey(k)
    const s = byKey.get(key)
    if (s && s.targets.includes(agent) !== want) next[k] = want
    else changed = true
  }
  return changed ? next : pending
}

export interface SkillChange {
  skill: Skill
  targets: string[]
  keys: string[]
}

/** One setTargets call per skill with pending changes, in list order. */
export function changes(pending: Pending, skills: Skill[]): SkillChange[] {
  const out: SkillChange[] = []
  for (const s of skills) {
    const keys = Object.keys(pending).filter((k) => splitKey(k)[0] === s.ref.key)
    if (!keys.length) continue
    const targets = new Set(s.targets)
    for (const k of keys) {
      const agent = splitKey(k)[1]
      if (pending[k]) targets.add(agent)
      else targets.delete(agent)
    }
    out.push({ skill: s, targets: [...targets], keys })
  }
  return out
}
