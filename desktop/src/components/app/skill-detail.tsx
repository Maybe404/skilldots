import * as React from "react"
import { Copy, FolderOpen, PencilLine, SquareArrowOutUpRight, Trash2, Unlink, X } from "lucide-react"

import { PLACEHOLDER_DESC, kindLabel, useApp, useEnabledAgents } from "@/lib/app-state"
import type { Skill } from "@/lib/backend"
import { useSkillActions } from "@/lib/use-actions"
import { Mono, OffRing, UpdateDot } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"

export function SkillDetail({ skill, onClose }: { skill: Skill; onClose: () => void }) {
  const { t, overrides, outdated, updatable, packs, openEditor, openDialog } = useApp()
  const agents = useEnabledAgents()
  const { setAgent, reveal, openInEditor } = useSkillActions()
  const ref = React.useRef<HTMLElement>(null)
  const status = outdated?.packs.find((p) => p.repo === skill.origin)
  const moved = status?.moved.find((m) => m.startsWith(skill.name + ":"))
  const pack = packs.find((p) => p.repo === skill.origin)
  const claudeOff = overrides.has(skill.name)

  React.useEffect(() => {
    ref.current?.focus()
  }, [skill.ref.key])

  return (
    <aside
      ref={ref}
      tabIndex={-1}
      aria-label={t("details")}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.stopPropagation()
          onClose()
        }
      }}
      className="flex w-[22rem] shrink-0 flex-col border-l bg-background outline-none"
    >
      <div className="flex items-start justify-between gap-3 px-5 pt-5 pb-3">
        <div className="min-w-0">
          <h2 className="truncate text-lg font-semibold tracking-tight">{skill.linkName}</h2>
          <p className="mt-0.5 truncate text-[0.8rem] text-muted-foreground">{kindLabel(t, skill)}</p>
        </div>
        <Button variant="ghost" size="icon-sm" onClick={onClose} aria-label={t("closeDetails")}>
          <X />
        </Button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-5 pb-5">
        {skill.description === PLACEHOLDER_DESC ? (
          <div className="rounded-lg border border-dashed p-3 text-[0.82rem] leading-relaxed">
            <p>{t("placeholderWarn", { name: skill.name })}</p>
            <Button variant="outline" size="sm" className="mt-2" onClick={() => openEditor(skill.ref)}>
              {t("writeDescription")}
            </Button>
          </div>
        ) : (
          <p className="text-[0.85rem] leading-relaxed">{skill.description}</p>
        )}

        {(updatable.has(skill.ref.key) || moved) && (
          <div className="mt-4 rounded-lg bg-muted p-3 text-[0.82rem]">
            {updatable.has(skill.ref.key) && (
              <>
                <p className="flex items-center gap-2 font-medium">
                  <UpdateDot decorative /> {t("upstreamChanged")}
                </p>
                <ul className="mt-1.5 space-y-0.5 text-muted-foreground">
                  {status?.commits.slice(0, 3).map((c) => (
                    <li key={c.id} className="truncate">
                      <Mono className="mr-1.5">{c.id}</Mono>
                      {c.title}
                    </li>
                  ))}
                </ul>
                <Button size="sm" className="mt-2.5" onClick={() => openDialog({ kind: "update", repos: [skill.origin] })}>
                  {t("reviewUpdates")}
                </Button>
              </>
            )}
            {moved && <p className="text-muted-foreground">{t("upstreamMoved")}: <Mono>{moved.split(": ")[1]}</Mono></p>}
          </div>
        )}

        <Separator className="my-5" />

        <h3 className="text-[0.8rem] font-semibold">{t("installedIn")}</h3>
        <p className="mt-0.5 text-[0.78rem] text-muted-foreground">{t("installedHint")}</p>
        <ul className="mt-3 space-y-1">
          {agents.map((a) => {
            const on = skill.targets.includes(a.name)
            const id = `agent-${a.name}`
            return (
              <li key={a.name} className="flex items-center justify-between gap-3 rounded-md py-1.5">
                <label htmlFor={id} className="flex min-w-0 items-center gap-2 text-[0.85rem]">
                  {a.label}
                  {a.name === "claude" && claudeOff && <OffRing />}
                </label>
                <Switch id={id} checked={on} onCheckedChange={(next) => setAgent(skill, a.name, next)} />
              </li>
            )
          })}
        </ul>
        {claudeOff && (
          <p className="mt-2 flex gap-2 text-[0.78rem] leading-relaxed text-muted-foreground">
            <OffRing className="mt-1" />
            <span>{t("claudeOffNote")}</span>
          </p>
        )}

        <Separator className="my-5" />

        <dl className="grid grid-cols-[5rem_1fr] gap-x-3 gap-y-2 text-[0.82rem]">
          <dt className="text-muted-foreground">{t("infoSource")}</dt>
          <dd className="min-w-0 truncate">{skill.kind === "vendor" ? skill.origin : kindLabel(t, skill)}</dd>
          {skill.category && (
            <>
              <dt className="text-muted-foreground">{t("infoCategory")}</dt>
              <dd className="min-w-0 truncate">{skill.category}</dd>
            </>
          )}
          {pack && (
            <>
              <dt className="text-muted-foreground">{t("infoCommit")}</dt>
              <dd>
                <Mono>{pack.commit}</Mono>
              </dd>
            </>
          )}
          <dt className="text-muted-foreground">{t("infoLinkName")}</dt>
          <dd>
            <Mono>{skill.linkName}</Mono>
          </dd>
          <dt className="text-muted-foreground">{t("infoLocation")}</dt>
          <dd className="min-w-0 break-all">
            <Mono className="text-muted-foreground">{skill.dir}</Mono>
          </dd>
        </dl>
      </div>

      <div className="space-y-2 border-t p-4">
        <Button className="w-full" onClick={() => openEditor(skill.ref)}>
          <PencilLine /> {t("edit")}
        </Button>
        <div className="grid grid-cols-2 gap-2">
          <Button variant="outline" onClick={() => reveal(skill.ref)}>
            <FolderOpen /> {t("revealInFinder")}
          </Button>
          <Button variant="outline" onClick={() => openInEditor(skill.ref)}>
            <SquareArrowOutUpRight /> {t("openInEditor")}
          </Button>
        </div>
        {(skill.kind === "vendor" || skill.kind === "app") && (
          <Button variant="outline" className="w-full" onClick={() => openDialog({ kind: "fork", ref: skill.ref })}>
            <Copy /> {t("forkSkill")}
          </Button>
        )}
        {skill.kind !== "app" ? (
          <Button variant="ghost" className="w-full" onClick={() => openDialog({ kind: "remove", ref: skill.ref })}>
            <Trash2 /> {t("moveToTrash")}
          </Button>
        ) : (
          <Button variant="ghost" className="w-full" disabled={skill.targets.length === 0} onClick={() => openDialog({ kind: "unlink", ref: skill.ref })}>
            <Unlink /> {t("unlinkAll")}
          </Button>
        )}
      </div>
    </aside>
  )
}
