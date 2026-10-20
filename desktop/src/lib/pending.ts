import type { Agent, Skill } from "@/lib/backend"

/*
 * Link changes the user has made but not applied yet (the design's "待应用").
 * The backend saves and applies every enable/disable call at once, so the
 * desktop collects the wanted state here and calls enable / disable per skill
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

/** Want every one of skills on (or off) for agent: a pending change where that differs from now, none where it does not. */
export function setMany(pending: Pending, skills: Skill[], agent: string, want: boolean): Pending {
  const next = { ...pending }
  for (const s of skills) {
    const k = pendingKey(s.ref.key, agent)
    if (s.targets.includes(agent) === want) delete next[k]
    else next[k] = want
  }
  return next
}

/**
 * Drops entries for skills that are gone or already in the wanted state, and
 * for agents whose Skill-page column is hidden (or gone): a change nobody can
 * see must not be applied.
 */
export function prune(pending: Pending, skills: Skill[], agents: Agent[]): Pending {
  const byKey = new Map(skills.map((s) => [s.ref.key, s]))
  const shown = new Set(agents.filter((a) => !a.hidden).map((a) => a.name))
  const next: Pending = {}
  let changed = false
  for (const [k, want] of Object.entries(pending)) {
    const [key, agent] = splitKey(k)
    const s = byKey.get(key)
    if (s && shown.has(agent) && s.targets.includes(agent) !== want) next[k] = want
    else changed = true
  }
  return changed ? next : pending
}

/** Pending changes on one agent's column. */
export function pendingOnAgent(pending: Pending, agent: string): number {
  return Object.keys(pending).filter((k) => splitKey(k)[1] === agent).length
}

export interface SkillChange {
  skill: Skill
  /** Agents to add to the skill's targets. */
  enable: string[]
  /** Agents to remove from the skill's targets. */
  disable: string[]
  /** Pending keys per direction, so a partial failure can tell what went through. */
  enableKeys: string[]
  disableKeys: string[]
}

/**
 * Per skill, the agents to add and the agents to remove, in list order.
 * The desktop sends these as enable / disable instead of one setTargets with
 * the whole list: the backend then edits the latest targets on disk, so a
 * change the CLI or another agent made to the same skill while this window
 * was open is not rolled back.
 */
export function changes(pending: Pending, skills: Skill[]): SkillChange[] {
  const out: SkillChange[] = []
  for (const s of skills) {
    const c: SkillChange = { skill: s, enable: [], disable: [], enableKeys: [], disableKeys: [] }
    for (const [k, want] of Object.entries(pending)) {
      const [key, agent] = splitKey(k)
      if (key !== s.ref.key) continue
      if (want) {
        c.enable.push(agent)
        c.enableKeys.push(k)
      } else {
        c.disable.push(agent)
        c.disableKeys.push(k)
      }
    }
    if (c.enable.length || c.disable.length) out.push(c)
  }
  return out
}
