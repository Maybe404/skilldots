import * as React from "react"
import { toast } from "sonner"
import { AnimatePresence, motion, useReducedMotion } from "motion/react"
import { ArrowLeft, Check, FileWarning, FolderOpen, GitMerge, PencilLine } from "lucide-react"

import { cn } from "@/lib/utils"
import { hasConflicts, kindLabel, toastAbsent, useApp } from "@/lib/app-state"
import { backend, type ConflictBlock, type ConflictFile, type Ref, type ResolveChoice, type ResolveState, type Skill } from "@/lib/backend"
import { useSkillActions } from "@/lib/use-actions"
import { EmptyNote, Hint, RefreshIcon, Tag } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Collapsible, CollapsiblePanel, CollapsibleTrigger, DisclosureChevron } from "@/components/ui/collapsible"
import { Skeleton } from "@/components/ui/skeleton"

/*
 * Resolving a shelved update (skd resolve), as a full page like the editor:
 * the conflicted files on the left; on the right each conflict of the open
 * file with the local and upstream sides and one button per way out. Picking
 * rewrites that block in the file (resolvePick); 手动编辑 opens the editor on
 * the file, markers and all. 完成合并 is offered once no markers remain;
 * 先搁置 puts the folder back as it was and keeps the update shelved.
 */

/** Conflicts left in a file: its blocks, or one for a binary file (resolved by hand in Finder). */
const left = (f: ConflictFile) => f.blocks.length + (f.binary ? 1 : 0)

/** "e52f8c0" from the file's ">>>>>>> upstream e52f8c0" marker, when it is there. */
const upstreamLabel = (f?: ConflictFile) => f?.text.match(/^>>>>>>> upstream (\S+)/m)?.[1]

export function ResolvePage({ skillRef }: { skillRef: Ref }) {
  const { skills } = useApp()
  const skill = skills.find((s) => s.ref.key === skillRef.key)
  if (!skill) return null
  if (!hasConflicts(skill)) return <NothingToResolve skill={skill} />
  return <Resolver skill={skill} />
}

function NothingToResolve({ skill }: { skill: Skill }) {
  const { t, navigate } = useApp()
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <TopBar skill={skill} onBack={() => navigate("skills")} />
      <div className="px-10 pt-10">
        <EmptyNote
          title={t("noConflicts", { name: skill.linkName })}
          action={
            <Button variant="outline" onClick={() => navigate("skills")}>
              {t("backToSkills")}
            </Button>
          }
        />
      </div>
    </div>
  )
}

function TopBar({ skill, onBack, children }: { skill: Skill; onBack: () => void; children?: React.ReactNode }) {
  const { t } = useApp()
  return (
    <div className="flex h-[46px] shrink-0 items-center gap-2 px-10 font-body fs-body max-[760px]:px-5">
      <Button variant="ghost" onClick={onBack} className="-ml-2.5">
        <ArrowLeft /> {t("back")}
      </Button>
      <div className="ml-1 flex min-w-0 items-baseline gap-2">
        <h1 className="truncate font-heading fs-name font-semibold text-ink">{t("resolveTitle", { name: skill.linkName })}</h1>
        <span className="truncate fs-meta text-mut max-[900px]:hidden">{kindLabel(t, skill)}</span>
      </div>
      {children}
    </div>
  )
}

function Resolver({ skill }: { skill: Skill }) {
  const { t, agents, run, runTask, read, reload, navigate, openEditor, confirm, operation } = useApp()
  const { reveal } = useSkillActions()
  const reduce = useReducedMotion()
  const [state, setState] = React.useState<ResolveState | null>(null)
  const [failed, setFailed] = React.useState(false)
  const [active, setActive] = React.useState<string | null>(null)
  const [busy, setBusy] = React.useState<string | null>(null)
  // Files in the order first seen: one the bridge stops listing once resolved stays in the list, marked resolved.
  const [order, setOrder] = React.useState<string[]>(() => skill.conflictFiles ?? [])
  const started = React.useRef(false)
  const ref = skill.ref

  const take = React.useCallback((s: ResolveState) => {
    setState(s)
    setOrder((o) => [...o, ...s.files.map((f) => f.path).filter((p) => !o.includes(p))])
    setActive((a) => a ?? s.files.find((f) => left(f) > 0)?.path ?? s.files[0]?.path ?? null)
  }, [])

  // Starting writes the merge with markers into the folder; coming back (from the editor, or later) only reads it.
  React.useEffect(() => {
    if (started.current) return
    started.current = true
    ;(async () => {
      if (skill.resolving) {
        const r = await read(() => backend.resolveState(ref))
        if (r.ok) take(r.value)
        else setFailed(true)
        return
      }
      const r = await runTask(t("resolveStarting"), () => backend.resolveStart(ref))
      if (!r.ok) {
        setFailed(true)
        return
      }
      take(r.value)
      await reload().catch(() => undefined)
    })()
  }, [skill.resolving, ref, read, runTask, reload, take, t])

  const refresh = React.useCallback(async () => {
    const r = await read(() => backend.resolveState(ref))
    if (r.ok) take(r.value)
  }, [read, ref, take])

  const pick = async (file: ConflictFile, block: ConflictBlock, choice: ResolveChoice) => {
    setBusy(`${file.path}#${block.index}`)
    const r = await run(() => backend.resolvePick(ref, file.path, block.index, choice))
    if (r.ok) await refresh()
    setBusy(null)
  }

  const finish = async () => {
    const r = await runTask(t("operating"), () => backend.resolveContinue(ref))
    await reload().catch(() => undefined)
    if (!r.ok) {
      await refresh()
      return
    }
    toast(t("mergedToast", { name: skill.linkName }))
    if (r.value) toastAbsent(t, agents, r.value.absent)
    navigate("skills")
  }

  const shelve = () =>
    confirm({
      title: t("shelveConfirmTitle", { name: skill.linkName }),
      body: t("shelveConfirmBody"),
      action: t("shelveAgain"),
      run: async () => {
        const r = await runTask(t("operating"), () => backend.resolveAbort(ref))
        if (!r.ok) return false
        await reload().catch(() => undefined)
        toast(t("shelvedAgainToast", { name: skill.linkName }))
        navigate("skills")
      },
    })

  const files = state?.files ?? []
  const byPath = new Map(files.map((f) => [f.path, f]))
  const remaining = files.reduce((n, f) => n + left(f), 0)
  const current = active ? byPath.get(active) : undefined
  const ready = !!state && remaining === 0

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <TopBar skill={skill} onBack={() => navigate("skills")}>
        {state && (
          <span className="flex shrink-0 items-center gap-1.5 fs-meta text-ink" role="status">
            {remaining > 0 ? (
              <>
                <span className="size-1.5 rounded-full bg-ac" aria-hidden />
                {t("conflictsLeft", { n: remaining })}
              </>
            ) : (
              <>
                <Check className="size-3.5" aria-hidden />
                {t("fileResolved")}
              </>
            )}
          </span>
        )}
        <div className="ml-auto flex items-center gap-1.5">
          <Hint label={t("shelveAgainHint")} wide>
            <Button variant="ghost" onClick={shelve} disabled={!state || !!operation}>
              {t("shelveAgain")}
            </Button>
          </Hint>
          <Button onClick={() => void finish()} disabled={!ready || !!operation} className="px-3">
            <GitMerge /> {t("finishMerge")}
          </Button>
        </div>
      </TopBar>

      <div className="flex items-center gap-2 border-b-row bg-row-hv px-10 py-1.5 fs-meta text-ink [border-left:3px_solid_var(--ac)] max-[760px]:px-5" role="note">
        {t("resolveBanner")}
      </div>

      <div className="flex min-h-0 flex-1">
        <nav aria-label={t("conflictFiles")} className="w-56 shrink-0 overflow-y-auto py-2 pl-7 [border-right:var(--tblRow)] lg:w-64 max-[760px]:w-44 max-[760px]:pl-3">
          <p className="px-2 pt-1 pb-1 fs-meta text-mut">{t("conflictFiles")}</p>
          {!state && !failed && <Skeleton className="mx-2 h-8" />}
          <ul className="space-y-0.5 pr-2">
            {order.map((path) => {
              const f = byPath.get(path)
              const n = f ? left(f) : 0
              return (
                <li key={path}>
                  <button
                    type="button"
                    onClick={() => setActive(path)}
                    aria-current={active === path ? "true" : undefined}
                    className={cn("flex w-full items-center gap-2 rounded-ctl px-2 py-1.5 text-left hover:bg-row-hv", active === path && "bg-foreground/[0.07]")}
                  >
                    <span className={cn("min-w-0 flex-1 truncate font-mono fs-meta", active === path ? "font-medium text-ink" : "text-ink")} title={path}>
                      {path}
                    </span>
                    {n > 0 ? (
                      <span className="shrink-0 fs-meta text-ac tabular-nums">{n}</span>
                    ) : (
                      <Check aria-label={t("fileResolved")} className="size-3.5 shrink-0 text-mut" />
                    )}
                  </button>
                </li>
              )
            })}
          </ul>
        </nav>

        <section aria-label={current?.path ?? t("conflictFiles")} className="min-w-0 flex-1 overflow-y-auto px-8 py-5 doodle:pb-14 max-[760px]:px-4">
          {failed && !state ? (
            <div role="alert" className="fs-body text-mut">
              <p className="mb-1 font-heading fs-h3 text-ink">{t("pageLoadFailed")}</p>
              <Button variant="outline" className="mt-3" onClick={() => navigate("skills")}>
                {t("backToSkills")}
              </Button>
            </div>
          ) : !state ? (
            <p className="flex items-center gap-2 fs-body text-mut" role="status">
              <RefreshIcon spinning /> {t("resolveStarting")}
            </p>
          ) : (
            <>
              {ready && (
                <div className="mb-4 flex flex-wrap items-center gap-3 rounded-ctl bg-hue-soft px-3.5 py-2.5 fs-body text-ink" role="status">
                  <Check className="size-4 shrink-0" aria-hidden />
                  <span className="min-w-0 flex-1">{t("allResolved")}</span>
                  <Button size="sm" onClick={() => void finish()} disabled={!!operation}>
                    <GitMerge /> {t("finishMerge")}
                  </Button>
                </div>
              )}
              {current && (
                <div className="mb-3 flex items-center gap-2">
                  <h2 className="min-w-0 truncate font-mono fs-body font-medium text-ink">{current.path}</h2>
                  {left(current) > 0 && <Tag>{t("conflictsLeft", { n: left(current) })}</Tag>}
                  <span className="flex-1" />
                  <Button variant="ghost" size="sm" onClick={() => openEditor(skill.ref, { file: current.path, back: "resolve" })}>
                    <PencilLine /> {t("editManually")}
                  </Button>
                </div>
              )}
              {current?.binary ? (
                <div className="flex items-start gap-3 rounded-ctl border-line p-3.5 fs-body text-ink">
                  <FileWarning className="mt-0.5 size-4 shrink-0 text-mut" aria-hidden />
                  <div className="min-w-0 flex-1">
                    <p className="leading-relaxed">{t("binaryConflict", { file: current.path })}</p>
                    <Button variant="outline" size="sm" className="mt-2.5" onClick={() => reveal(skill.ref)}>
                      <FolderOpen /> {t("revealInFinder")}
                    </Button>
                  </div>
                </div>
              ) : current && current.blocks.length === 0 ? (
                <p className="fs-body text-mut">{t("fileAllResolved")}</p>
              ) : !current && active ? (
                <p className="fs-body text-mut">{t("fileAllResolved")}</p>
              ) : null}
              <div className="space-y-4">
                <AnimatePresence initial={false} mode="popLayout">
                  {current?.blocks.map((b) => (
                    <motion.div
                      // By content: when a conflict above is resolved, the ones below keep their card (their index changes).
                      key={`${current.path}|${b.local}|${b.upstream}`}
                      layout={!reduce}
                      initial={reduce ? false : { opacity: 0, y: 6 }}
                      animate={{ opacity: 1, y: 0 }}
                      exit={reduce ? { opacity: 0, transition: { duration: 0 } } : { opacity: 0, scale: 0.98, transition: { duration: 0.18 } }}
                      transition={{ type: "spring", stiffness: 420, damping: 36 }}
                    >
                      <Block
                        block={b}
                        total={current.blocks.length}
                        upstream={upstreamLabel(current)}
                        busy={busy === `${current.path}#${b.index}`}
                        disabled={!!busy || !!operation}
                        onPick={(c) => void pick(current, b, c)}
                        onEdit={() => openEditor(skill.ref, { file: current.path, back: "resolve" })}
                      />
                    </motion.div>
                  ))}
                </AnimatePresence>
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  )
}

function Block({
  block,
  total,
  upstream,
  busy,
  disabled,
  onPick,
  onEdit,
}: {
  block: ConflictBlock
  total: number
  upstream?: string
  busy: boolean
  disabled: boolean
  onPick: (c: ResolveChoice) => void
  onEdit: () => void
}) {
  const { t } = useApp()
  const [baseOpen, setBaseOpen] = React.useState(false)
  const id = `conflict-${block.index}`
  return (
    <section aria-labelledby={id} className="overflow-hidden rounded-card border-line bg-bg">
      <header className="flex h-9 items-center gap-2 px-3.5 [border-bottom:1.5px_solid_var(--ln)]">
        <span className="size-1.5 rounded-full bg-ac" aria-hidden />
        <h3 id={id} className="fs-meta font-semibold text-ink">
          {t("conflictOf", { i: block.index + 1, n: total })}
        </h3>
        {busy && <RefreshIcon spinning className="ml-1" />}
      </header>
      <div className="grid lg:grid-cols-2">
        <Side label={t("sideLocal")} text={block.local} />
        <Side label={t("sideUpstream")} sha={upstream} text={block.upstream} className="[border-top:1.5px_solid_var(--ln)] lg:[border-top:0] lg:[border-left:1.5px_solid_var(--ln)]" />
      </div>
      {block.base !== undefined && (
        <Collapsible open={baseOpen} onOpenChange={setBaseOpen} className="[border-top:1px_solid_var(--ln)]">
          <CollapsibleTrigger className="flex h-8 w-full items-center gap-1.5 px-3.5 text-left fs-meta text-mut hover:bg-row-hv hover:text-ink">
            <DisclosureChevron open={baseOpen} />
            {t("sideBase")}
          </CollapsibleTrigger>
          <CollapsiblePanel>
            <Code text={block.base} className="px-3.5 pb-3 text-mut" />
          </CollapsiblePanel>
        </Collapsible>
      )}
      <footer className="flex flex-wrap items-center gap-1.5 bg-nav px-3.5 py-2.5 [border-top:1.5px_solid_var(--ln)]">
        <Button variant="outline" size="sm" disabled={disabled} onClick={() => onPick("local")}>
          {t("useMine")}
        </Button>
        <Button variant="outline" size="sm" disabled={disabled} onClick={() => onPick("upstream")}>
          {t("useUpstream")}
        </Button>
        <Button variant="outline" size="sm" disabled={disabled} onClick={() => onPick("both")}>
          {t("useBoth")}
        </Button>
        <span className="flex-1" />
        <Button variant="ghost" size="sm" disabled={disabled} onClick={onEdit}>
          <PencilLine /> {t("editManually")}
        </Button>
      </footer>
    </section>
  )
}

function Side({ label, sha, text, className }: { label: string; sha?: string; text: string; className?: string }) {
  return (
    <div className={cn("min-w-0", className)}>
      <p className="flex items-center gap-1.5 px-3.5 pt-2.5 pb-1 fs-meta font-medium text-mut">
        {label}
        {sha && <span className="font-mono font-normal">{sha}</span>}
      </p>
      <Code text={text} className="px-3.5 pb-3 text-ink" />
    </div>
  )
}

/** One side of a conflict, in the editor's code font (diff text only). */
function Code({ text, className }: { text: string; className?: string }) {
  const { t } = useApp()
  if (!text) return <p className={cn("fs-meta italic", className)}>{t("emptySide")}</p>
  return <pre className={cn("m-0 max-h-[18rem] overflow-auto font-[family-name:var(--fmCode)] text-[11.5px] leading-[1.6] break-words whitespace-pre-wrap", className)}>{text}</pre>
}
