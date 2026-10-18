import * as React from "react"
import { isMine, sourceGroup, useApp, useColumnAgents, type SkillFilter, type TableSort } from "@/lib/app-state"
import type { Skill } from "@/lib/backend"
import { isOn, type Pending } from "@/lib/pending"

/* Geometry of the wire view, from the design (px). */
export const ROW_PITCH = 58
export const ROW_H = 50
export const GROUP_H = 34
/** Top of the first row inside the list column (12 margin + 24 header + 6). */
export const LIST_TOP = 42
export const AGENT_H = 72
/** Top of the first agent row (12 padding + 24 header + 6). */
export const AGENTS_TOP = 42

export function matchesFilter(s: Skill, filter: SkillFilter, updatable: Set<string>) {
  switch (filter) {
    case "mine":
      return isMine(s)
    case "vendor":
      return s.kind === "vendor" || s.kind === "app"
    case "updates":
      return updatable.has(s.ref.key)
    default:
      return true
  }
}

export function matchesQuery(s: Skill, q: string) {
  if (!q) return true
  const n = q.toLowerCase()
  return [s.linkName, s.name, s.origin, s.description, s.category].some((x) => x.toLowerCase().includes(n))
}

export interface SkillItem {
  type: "skill"
  skill: Skill
  /** Top of the row in the list column. */
  y: number
  /** Not installed in the focused agent. */
  dim: boolean
}

export interface GroupItem {
  type: "group"
  name: string
  y: number
  collapsed: boolean
  skills: Skill[]
}

export type ListItem = SkillItem | GroupItem

function sorted(list: Skill[], sort: TableSort, source: (s: Skill) => string) {
  if (!sort) return list
  const key = sort.by === "name" ? (s: Skill) => s.linkName : source
  const out = [...list].sort((a, b) => key(a).localeCompare(key(b)) || a.linkName.localeCompare(b.linkName))
  return sort.dir === "desc" ? out.reverse() : out
}

/** The visible skills of the Skill page, in display order, with wire-view positions. */
export function useSkillList() {
  const { t, skills, updatable, query, filter, grouped, collapsed, agentFocus, pending, tableSort } = useApp()
  const columns = useColumnAgents()

  return React.useMemo(() => {
    const q = query.trim()
    const source = (s: Skill) => sourceGroup(t, s)
    let vis = sorted(
      skills.filter((s) => matchesFilter(s, filter, updatable) && matchesQuery(s, q)),
      tableSort,
      source,
    )
    if (agentFocus) vis = [...vis.filter((s) => isOn(s, agentFocus, pending)), ...vis.filter((s) => !isOn(s, agentFocus, pending))]
    const dim = (s: Skill) => !!agentFocus && !isOn(s, agentFocus, pending)

    const items: ListItem[] = []
    const order: Skill[] = []
    let y = 0
    const push = (s: Skill) => {
      items.push({ type: "skill", skill: s, y, dim: dim(s) })
      order.push(s)
      y += ROW_PITCH
    }
    if (grouped) {
      const groups: { name: string; list: Skill[] }[] = []
      for (const s of vis) {
        const name = source(s)
        let g = groups.find((x) => x.name === name)
        if (!g) groups.push((g = { name, list: [] }))
        g.list.push(s)
      }
      for (const g of groups) {
        const c = collapsed.has(g.name)
        items.push({ type: "group", name: g.name, y, collapsed: c, skills: g.list })
        y += GROUP_H
        if (!c) g.list.forEach(push)
      }
    } else vis.forEach(push)

    return { items, order, count: vis.length, listHeight: y + 12, columns }
  }, [t, skills, updatable, query, filter, grouped, collapsed, agentFocus, pending, tableSort, columns])
}

/** Installed agents of a skill (after pending changes), as a sentence fragment. */
export function installedCount(s: Skill, columns: { name: string }[], pending: Pending) {
  return columns.filter((a) => isOn(s, a.name, pending)).length
}
