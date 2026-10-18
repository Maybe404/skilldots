import * as React from "react"
import { Copy, FolderOpen, PencilLine, SquareArrowOutUpRight, Trash2, Unlink } from "lucide-react"

import { PLACEHOLDER_DESC, isMine, kindLabel, useApp, useColumnAgents } from "@/lib/app-state"
import { agentLabel } from "@/lib/agents"
import type { Skill } from "@/lib/backend"
import { cellState } from "@/lib/pending"
import { useSkillActions } from "@/lib/use-actions"
import { AgentMono, Mono, OffRing, Tag, UpdateDot } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Switch } from "@/components/ui/switch"

/**
 * Skill details (not drawn in the design): a side sheet over the Skill page
 * with the description, where it is installed, upstream changes and the
 * actions that used to live in the old detail panel. Agent switches add
 * pending changes like everything else on the Skill page.
 */
export function SkillSheet({ returnFocus }: { returnFocus: () => HTMLElement | null }) {
  const { t, detail, closeDetail, skills } = useApp()
  const skill = skills.find((s) => s.ref.key === detail) ?? null
  const start = React.useRef<HTMLDivElement>(null)
  return (
    <Sheet open={!!skill} onOpenChange={(o) => !o && closeDetail()}>
      {/*
        Focus lands on the panel body, not on its first switch: the key that
        opened the panel (Space) would otherwise toggle that switch on keyup.
        Closing returns focus to the skill's row.
      */}
      <SheetContent side="right" className="w-[400px]" closeLabel={t("close")} initialFocus={start} finalFocus={() => returnFocus() ?? true}>
        {skill && <Body skill={skill} start={start} />}
      </SheetContent>
    </Sheet>
  )
}

function Body({ skill, start }: { skill: Skill; start: React.RefObject<HTMLDivElement | null> }) {
  const { t, overrides, outdated, updatable, packs, openEditor, openDialog, pending, togglePending, pendingCount, applyPending, showSource, operation } = useApp()
  const columns = useColumnAgents()
  const { reveal, openInEditor } = useSkillActions()
  const status = outdated?.packs.find((p) => p.repo === skill.origin)
  const moved = status?.moved.find((m) => m.startsWith(skill.name + ":"))
  const pack = packs.find((p) => p.repo === skill.origin)
  const claudeOff = overrides.has(skill.name)
  const mine = isMine(skill)

  return (
    <>
      <SheetHeader>
        <SheetTitle className="flex items-center gap-2">
          <span className="truncate">{skill.linkName}</span>
          {updatable.has(skill.ref.key) && <UpdateDot />}
          {mine && <Tag>{skill.kind === "private" ? t("kindPrivate") : t("kindMine")}</Tag>}
        </SheetTitle>
        <SheetDescription className="truncate font-mono">{kindLabel(t, skill)}</SheetDescription>
      </SheetHeader>

      <div ref={start} tabIndex={-1} className="min-h-0 flex-1 overflow-y-auto px-6 pb-6 outline-none">
        {skill.description === PLACEHOLDER_DESC ? (
          <div className="rounded-ctl border-[1.5px] border-dashed border-ac p-3 fs-body leading-relaxed">
            <p>{t("placeholderWarn", { name: skill.name })}</p>
            <Button variant="outline" size="sm" className="mt-2.5" onClick={() => openEditor(skill.ref)}>
              {t("writeDescription")}
            </Button>
          </div>
        ) : (
          <p className="fs-body leading-relaxed text-pretty text-ink">{skill.description}</p>
        )}

        {(updatable.has(skill.ref.key) || moved) && (
          <div className="mt-4 rounded-ctl bg-row-hv p-3 fs-meta [border-left:3px_solid_var(--ac)]">
            {updatable.has(skill.ref.key) && (
              <>
                <p className="flex items-center gap-2 font-semibold text-ink">
                  {t("upstreamChanged")}
                  {status && (
                    <Mono className="font-normal text-mut">
                      {status.commit} → {status.latest}
                    </Mono>
                  )}
                </p>
                <ul className="mt-1.5 space-y-1 text-ink">
                  {status?.commits.slice(0, 3).map((c) => (
                    <li key={c.id} className="flex gap-3">
                      <Mono className="w-16 shrink-0 text-mut">{c.id}</Mono>
                      <span className="min-w-0 truncate">{c.title}</span>
                    </li>
                  ))}
                </ul>
                <Button size="sm" className="mt-2.5" onClick={() => showSource(skill.origin)}>
                  {t("reviewUpdates")}
                </Button>
              </>
            )}
            {moved && (
              <p className="mt-1.5 text-mut">
                {t("upstreamMoved")}: <Mono>{moved.split(": ")[1]}</Mono>
              </p>
            )}
          </div>
        )}

        <h3 className="mt-6 flex h-[30px] items-center border-b-head fs-meta font-normal text-mut">{t("installedIn")}</h3>
        <ul>
          {columns.map((a) => {
            const st = cellState(skill, a.name, pending)
            const id = `sheet-agent-${a.name}`
            return (
              <li key={a.name} className="flex h-12 items-center gap-3 border-b-row">
                <AgentMono agent={a} size={26} />
                <label htmlFor={id} className="flex min-w-0 flex-1 items-center gap-2 fs-body text-ink">
                  <span className="truncate">{agentLabel(a)}</span>
                  {!a.enabled && <Tag>{t("pausedTag")}</Tag>}
                  {a.name === "claude" && claudeOff && <OffRing />}
                  {(st === "adding" || st === "removing") && <span className="fs-meta text-ac">{st === "adding" ? t("wireAdding") : t("wireRemoving")}</span>}
                </label>
                <Switch id={id} checked={st === "on" || st === "adding"} onCheckedChange={() => togglePending(skill, a.name)} />
              </li>
            )
          })}
        </ul>
        <p className="mt-2 fs-meta leading-relaxed text-mut">{t("installedHint")}</p>
        {claudeOff && (
          <p className="mt-2 flex gap-2 fs-meta leading-relaxed text-mut">
            <OffRing className="mt-1" />
            <span>{t("claudeOffNote")}</span>
          </p>
        )}
        {pendingCount > 0 && (
          <div className="mt-3 flex items-center gap-3 fs-body text-ink">
            <span aria-hidden className="size-[7px] rounded-full bg-ac" />
            {t("pendingN", { n: pendingCount })}
            <Button variant="accent" size="default" className="ml-auto" disabled={!!operation} onClick={() => void applyPending()}>
              {t("apply")}
            </Button>
          </div>
        )}

        <h3 className="mt-6 flex h-[30px] items-center border-b-head fs-meta font-normal text-mut">{t("details")}</h3>
        <dl className="grid grid-cols-[5.5rem_minmax(0,1fr)] gap-x-3 gap-y-2 pt-3 fs-meta">
          <dt className="text-mut">{t("infoSource")}</dt>
          <dd className="min-w-0 truncate text-ink">{kindLabel(t, skill)}</dd>
          {skill.category && (
            <>
              <dt className="text-mut">{t("infoCategory")}</dt>
              <dd className="min-w-0 truncate text-ink">{skill.category}</dd>
            </>
          )}
          {pack && (
            <>
              <dt className="text-mut">{t("infoCommit")}</dt>
              <dd>
                <Mono className="text-ink">{pack.commit}</Mono>
              </dd>
            </>
          )}
          <dt className="text-mut">{t("infoLinkName")}</dt>
          <dd>
            <Mono className="text-ink">{skill.linkName}</Mono>
          </dd>
          <dt className="text-mut">{t("infoLocation")}</dt>
          <dd className="min-w-0 break-all">
            <Mono className="text-mut">{skill.dir}</Mono>
          </dd>
        </dl>
      </div>

      <div className="grid gap-2 px-6 py-4 [border-top:var(--tblRow)]">
        <Button size="lg" onClick={() => openEditor(skill.ref)}>
          <PencilLine /> {t("edit")}
        </Button>
        <div className="grid grid-cols-2 gap-2">
          <Button variant="outline" size="lg" onClick={() => reveal(skill.ref)}>
            <FolderOpen /> {t("revealInFinder")}
          </Button>
          <Button variant="outline" size="lg" onClick={() => openInEditor(skill.ref)}>
            <SquareArrowOutUpRight /> {t("openInEditor")}
          </Button>
        </div>
        {(skill.kind === "vendor" || skill.kind === "app") && (
          <Button variant="outline" size="lg" onClick={() => openDialog({ kind: "fork", ref: skill.ref })}>
            <Copy /> {t("forkSkill")}
          </Button>
        )}
        {skill.kind !== "app" ? (
          <Button variant="ghost" size="lg" onClick={() => openDialog({ kind: "remove", ref: skill.ref })}>
            <Trash2 /> {t("moveToTrash")}
          </Button>
        ) : (
          <Button variant="ghost" size="lg" disabled={skill.targets.length === 0} onClick={() => openDialog({ kind: "unlink", ref: skill.ref })}>
            <Unlink /> {t("unlinkAll")}
          </Button>
        )}
      </div>
    </>
  )
}
