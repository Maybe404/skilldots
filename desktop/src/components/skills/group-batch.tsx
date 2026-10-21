import { ListChecks } from "lucide-react"

import { useApp, useColumnAgents } from "@/lib/app-state"
import { agentLabel, isAbsentAgent } from "@/lib/agents"
import type { Agent, Skill } from "@/lib/backend"
import { isOn } from "@/lib/pending"
import { Hint } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuGroup, DropdownMenuLabel, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"

/*
 * Batch switches of a "by source" group: install or remove every skill of
 * the group for one agent. They only record pending changes, like every
 * other switch on the Skill page; 应用 in the status bar writes them.
 */

/** How many of skills are installed in agent once the pending changes apply. */
export function groupState(skills: Skill[], agent: string, pending: Parameters<typeof isOn>[2]) {
  const n = skills.filter((s) => isOn(s, agent, pending)).length
  return { n, all: n === skills.length && n > 0, some: n > 0 && n < skills.length }
}

/** One agent's batch switch for a group (the table view's group row). */
export function GroupCell({ group, skills, agent }: { group: string; skills: Skill[]; agent: Agent }) {
  const { t, pending, setPendingMany } = useApp()
  const { n, all, some } = groupState(skills, agent.name, pending)
  const label = n === 0 && isAbsentAgent(agent) ? t("absentCannotInstall", { agent: agentLabel(agent) }) : all ? t("groupAllOff", { group, agent: agentLabel(agent) }) : t("groupAllOn", { group, agent: agentLabel(agent) })
  return (
    <Hint label={`${label} · ${t("groupBatchSome", { n, total: skills.length })}`}>
      <Checkbox aria-label={label} checked={all} indeterminate={some} disabled={n === 0 && isAbsentAgent(agent)} onCheckedChange={() => setPendingMany(skills, agent.name, !all)} />
    </Hint>
  )
}

/** All agents' batch switches for a group, in a menu (the wire view's group row has no columns). */
export function GroupBatchMenu({ group, skills }: { group: string; skills: Skill[] }) {
  const { t, pending, setPendingMany } = useApp()
  const columns = useColumnAgents()
  return (
    <DropdownMenu>
      <Hint label={t("groupBatch")}>
        <DropdownMenuTrigger render={<Button variant="ghost" size="icon-xs" aria-label={`${group} · ${t("groupBatch")}`} className="relative z-[1]" />}>
          <ListChecks aria-hidden className="size-3.5" strokeWidth={1.8} />
        </DropdownMenuTrigger>
      </Hint>
      <DropdownMenuContent align="start" className="w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel>{t("groupBatch")}</DropdownMenuLabel>
          {columns.map((a) => {
            const { n, all } = groupState(skills, a.name, pending)
            return (
              <DropdownMenuCheckboxItem
                key={a.name}
                checked={all}
                disabled={n === 0 && isAbsentAgent(a)}
                closeOnClick={false}
                aria-label={all ? t("groupAllOff", { group, agent: agentLabel(a) }) : t("groupAllOn", { group, agent: agentLabel(a) })}
                onCheckedChange={() => setPendingMany(skills, a.name, !all)}
              >
                <span className="flex-1 truncate">{agentLabel(a)}</span>
                <span className="fs-meta text-mut tabular-nums">
                  {n}/{skills.length}
                </span>
              </DropdownMenuCheckboxItem>
            )
          })}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
