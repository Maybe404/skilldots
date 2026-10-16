import * as React from "react"
import { toast } from "sonner"
import { ChevronRight } from "lucide-react"

import { useApp } from "@/lib/app-state"
import { backend, type Agent } from "@/lib/backend"
import { Hint, Mono, PageHeader, Section } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"

export function AgentsPage() {
  const { t, agents, skills, run, reload } = useApp()
  const enabled = agents.filter((a) => a.enabled)
  const detected = agents.filter((a) => !a.enabled && a.detected)
  const missing = agents.filter((a) => !a.enabled && !a.detected)
  const count = (a: Agent) => skills.filter((s) => s.targets.includes(a.name)).length

  const setDefault = async (a: Agent, on: boolean) => {
    const next = agents.filter((x) => (x.name === a.name ? on : x.isDefault)).map((x) => x.name)
    if ((await run(() => backend.setDefaultAgents(next))).ok) {
      await reload()
      toast(t("defaultsSaved"))
    }
  }
  const setEnabled = async (a: Agent, on: boolean) => {
    if ((await run(() => backend.setAgentEnabled(a.name, on))).ok) {
      await reload()
      toast(on ? t("agentEnabled", { agent: a.label }) : t("agentDisabled", { agent: a.label }))
    }
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <PageHeader title={t("agentsTitle")} hint={t("agentsHint")} />

      <Section title={t("enabledAgents")} hint={t("defaultInstallHint")}>
        <ul className="divide-y rounded-xl border">
          {enabled.map((a) => {
            const n = count(a)
            const id = `default-${a.name}`
            return (
              <li key={a.name} className="grid grid-cols-[1fr_auto_auto] items-center gap-5 px-4 py-3">
                <div className="min-w-0">
                  <div className="font-medium">{a.label}</div>
                  <Mono className="text-muted-foreground">{a.path}</Mono>
                </div>
                <span className="text-[0.8rem] text-muted-foreground tabular-nums">{t("skillsInAgent", { n })}</span>
                <div className="flex items-center gap-4">
                  <label htmlFor={id} className="flex items-center gap-2 text-[0.82rem]">
                    <Checkbox id={id} checked={a.isDefault} onCheckedChange={(v) => setDefault(a, !!v)} disabled={a.isDefault && agents.filter((x) => x.isDefault).length === 1} />
                    {t("defaultInstall")}
                  </label>
                  {n > 0 ? (
                    <Hint label={t("disableAgentBlocked", { n })}>
                      <span tabIndex={0} className="rounded-md">
                        <Button variant="ghost" size="sm" disabled>
                          {t("disableAgent")}
                        </Button>
                      </span>
                    </Hint>
                  ) : (
                    <Button variant="ghost" size="sm" onClick={() => setEnabled(a, false)}>
                      {t("disableAgent")}
                    </Button>
                  )}
                </div>
              </li>
            )
          })}
        </ul>
      </Section>

      <Section title={t("otherDetected")} hint={t("otherDetectedHint")}>
        {detected.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("noOtherAgents")}</p>
        ) : (
          <ul className="divide-y rounded-xl border">
            {detected.map((a) => (
              <li key={a.name} className="flex items-center justify-between gap-4 px-4 py-3">
                <div className="min-w-0">
                  <div className="font-medium">{a.label}</div>
                  <Mono className="text-muted-foreground">{a.path}</Mono>
                </div>
                <Button variant="outline" size="sm" onClick={() => setEnabled(a, true)}>
                  {t("enable")}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section title={t("notDetected")}>
        <NotDetected agents={missing} />
      </Section>
    </div>
  )
}

function NotDetected({ agents }: { agents: Agent[] }) {
  const [open, setOpen] = React.useState(false)
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger render={<Button variant="ghost" size="sm" className="-ml-2 text-muted-foreground" />}>
        <ChevronRight className={open ? "rotate-90" : undefined} />
        {agents.map((a) => a.label).slice(0, 4).join("、")}
        {agents.length > 4 && ` +${agents.length - 4}`}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <ul className="mt-2 grid grid-cols-2 gap-x-6 gap-y-1.5 text-[0.82rem] lg:grid-cols-3">
          {agents.map((a) => (
            <li key={a.name} className="min-w-0">
              <span>{a.label}</span> <Mono className="text-muted-foreground">{a.path}</Mono>
            </li>
          ))}
        </ul>
      </CollapsibleContent>
    </Collapsible>
  )
}
