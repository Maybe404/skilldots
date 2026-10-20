import * as React from "react"
import { toast } from "sonner"

import { cn } from "@/lib/utils"
import { useApp } from "@/lib/app-state"
import { agentLabel, tildify } from "@/lib/agents"
import { backend, type Agent } from "@/lib/backend"
import { isOn, pendingOnAgent } from "@/lib/pending"
import { AgentMono, Hint, Page, TableHead, Tag } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Collapsible, CollapsiblePanel, CollapsibleTrigger, DisclosureChevron } from "@/components/ui/collapsible"

const COLS = "var(--cols)"

/** Agents: default install, Skill-page column, pause / resume distribution. */
export function AgentsPage() {
  const { t, agents, skills, pending } = useApp()
  const [showOthers, setShowOthers] = React.useState(false)
  const count = (a: Agent) => skills.filter((s) => isOn(s, a.name, pending)).length
  const main = agents.filter((a) => !a.hidden || a.detected || a.isDefault || !a.enabled || count(a) > 0)
  const others = agents.filter((a) => !main.includes(a))
  const defaults = agents.filter((a) => a.isDefault).map(agentLabel)

  return (
    <Page title={t("agentsTitle")} summary={t("agentsSummary", { n: main.length, list: defaults.join(t("listSep")) || t("none") })}>
      <div role="table" aria-label={t("agentsTitle")} className="skd-cols-agents">
        <TableHead style={{ gridTemplateColumns: COLS }}>
          <div role="columnheader" />
          <div role="columnheader">Agent</div>
          <div role="columnheader">{t("colSkillsDir")}</div>
          <div role="columnheader">skill</div>
          <div role="columnheader" title={t("defaultInstallHint")}>
            {t("colDefault")}
          </div>
          <div role="columnheader" title={t("colColumnHint")}>
            {t("colColumn")}
          </div>
          <div role="columnheader" title={t("colDistributeHint")}>
            {t("colDistribute")}
          </div>
        </TableHead>
        {main.map((a) => (
          <AgentRow key={a.name} agent={a} count={count(a)} />
        ))}
      </div>
      <p className="mt-4 max-w-[640px] fs-meta text-pretty text-mut">{t("agentsFootnote")}</p>

      {others.length > 0 && (
        <Collapsible open={showOthers} onOpenChange={setShowOthers} className="mt-8">
          <CollapsibleTrigger className="flex h-[30px] items-center gap-2 rounded-seg fs-meta text-mut hover:text-ink">
            <DisclosureChevron open={showOthers} />
            {t("otherAgents", { n: others.length })}
          </CollapsibleTrigger>
          <CollapsiblePanel>
            <div role="table" aria-label={t("otherAgents", { n: others.length })} className="skd-cols-agents [border-top:var(--tblHd)]">
              {others.map((a) => (
                <AgentRow key={a.name} agent={a} count={count(a)} />
              ))}
            </div>
          </CollapsiblePanel>
        </Collapsible>
      )}
    </Page>
  )
}

function AgentRow({ agent: a, count }: { agent: Agent; count: number }) {
  const { t, agents, run, reload, confirm, pending } = useApp()
  const label = agentLabel(a)
  const defaults = agents.filter((x) => x.isDefault)
  const lastDefault = a.isDefault && defaults.length === 1
  const id = `default-${a.name}`

  const setDefault = async (on: boolean) => {
    const next = agents.filter((x) => (x.name === a.name ? on : x.isDefault)).map((x) => x.name)
    if ((await run(() => backend.setDefaultAgents(next))).ok) {
      await reload()
      toast(t("defaultsSaved"))
    }
  }
  const setHidden = async (hidden: boolean) => {
    if ((await run(() => backend.setAgentHidden(a.name, hidden))).ok) {
      await reload()
      toast(hidden ? t("columnHidden", { agent: label }) : t("columnShown", { agent: label }))
    }
  }
  // Pending changes in a hidden column would be applied unseen: ask first, and
  // drop them with the column (app-state prunes pending entries of hidden agents).
  const hide = (hidden: boolean) => {
    const n = hidden ? pendingOnAgent(pending, a.name) : 0
    if (!n) return void setHidden(hidden)
    confirm({
      title: t("hidePendingTitle", { agent: label }),
      body: t("hidePendingBody", { agent: label, n }),
      action: t("hidePendingAction"),
      run: () => setHidden(true),
    })
  }
  const setEnabled = async (on: boolean) => {
    if ((await run(() => backend.setAgentEnabled(a.name, on))).ok) {
      await reload()
      toast(on ? t("agentResumed", { agent: label, n: count }) : t("agentPaused", { agent: label }))
    }
  }

  return (
    <div role="row" className={cn("grid h-[57px] items-center border-b-row px-2.5 hover:bg-row-hv", a.hidden && "opacity-55")} style={{ gridTemplateColumns: COLS }}>
      <div role="cell">
        <AgentMono agent={a} size={28} />
      </div>
      <div role="cell" className="min-w-0 pr-3">
        <div className="flex min-w-0 items-center gap-2 font-heading fs-name leading-[1.2] font-semibold text-ink">
          <span className="truncate">{label}</span>
          {!a.enabled && <Tag tone="accent">{t("pausedLong")}</Tag>}
          {!a.detected && <Tag>{t("notOnMachine")}</Tag>}
        </div>
        {/* ~/.agents/skills is not one agent's folder: say who reads it (the full list on hover). */}
        {a.name === "agents" && (
          <Hint label={t("sharedDirFull")} wide>
            <p tabIndex={0} className="mt-0.5 w-fit max-w-full cursor-help truncate rounded-seg fs-meta text-mut">
              {t("sharedDirNote")}
            </p>
          </Hint>
        )}
        {/* An agent that reads the shared folder gets Agents' skills from there, not a link of its own. */}
        {a.readsShared && (
          <Hint label={t("viaSharedHint", { agent: label })}>
            <p tabIndex={0} className="mt-0.5 w-fit max-w-full cursor-help truncate rounded-seg fs-meta text-mut">
              {t("viaShared")}
            </p>
          </Hint>
        )}
      </div>
      <div role="cell" className="truncate pr-3 font-mono fs-meta text-mut" title={a.path}>
        {tildify(a.path)}
      </div>
      <div role="cell" className="fs-body text-hue-ink tabular-nums">
        {count}
      </div>
      <div role="cell">
        <label htmlFor={id} className="flex cursor-pointer items-center gap-2 fs-meta whitespace-nowrap text-ink" title={lastDefault ? t("lastDefault") : undefined}>
          <Checkbox id={id} checked={a.isDefault} disabled={lastDefault} onCheckedChange={(v) => void setDefault(!!v)} />
          {a.isDefault ? t("defaultOn") : t("defaultOff")}
        </label>
      </div>
      <div role="cell">
        <Button variant={a.hidden ? "link" : "quiet"} size="inline" className="fs-meta" onClick={() => hide(!a.hidden)} aria-label={`${label} · ${a.hidden ? t("showAgentColumn") : t("hideAgentColumn")}`}>
          {a.hidden ? t("hiddenShow") : t("hideAgentColumn")}
        </Button>
      </div>
      <div role="cell">
        <Button
          variant="quiet"
          size="inline"
          className={cn("fs-meta", !a.enabled && "font-semibold text-ac")}
          aria-label={`${label} · ${a.enabled ? t("pauseAgent") : t("resumeAgent")}`}
          onClick={() =>
            a.enabled
              ? confirm({
                  title: t("pauseTitle", { agent: label }),
                  body: t("pauseBody", { agent: label, n: count }),
                  action: t("pauseAgent"),
                  run: () => setEnabled(false),
                })
              : void setEnabled(true)
          }
        >
          {a.enabled ? t("pauseAgent") : t("resumeAgent")}
        </Button>
      </div>
    </div>
  )
}
