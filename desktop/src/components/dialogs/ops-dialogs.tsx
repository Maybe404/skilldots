import * as React from "react"
import { toast } from "sonner"
import { ArrowRight } from "lucide-react"

import { cn } from "@/lib/utils"
import { errorText, hasUpdates, toastAbsent, toastMergeOutcome, useApp } from "@/lib/app-state"
import { backend, type CliStatus, type PackStatus, type PlanItem, type SkillDiff, type UpdateDiff } from "@/lib/backend"
import type { MessageKey } from "@/lib/i18n"
import { Mono, RefreshIcon, Sha, Tag, UpdateDot } from "@/components/app/bits"
import { SkillDiffView } from "@/components/app/diff-view"
import { Collapsible, CollapsiblePanel, CollapsibleTrigger, DisclosureChevron } from "@/components/ui/collapsible"
import { agentLabel, tildify } from "@/lib/agents"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"

// Update ---------------------------------------------------------------------

export type DiffState = { state: "loading" } | { state: "ok"; diff: Pick<UpdateDiff, "skills"> } | { state: "error"; error: string }

/**
 * Reads, for each source, what updating it would change file by file
 * (read-only; the cache is reused when it is at the checked commit), so the
 * skills you edited are marked before anything happens.
 */
export function useUpdateDiffs(packs: PackStatus[]) {
  const { t } = useApp()
  const [diffs, setDiffs] = React.useState<Record<string, DiffState>>({})
  const key = packs.map((p) => p.repo + "@" + p.latest).join(",")
  React.useEffect(() => {
    let live = true
    for (const p of packs) {
      setDiffs((d) => ({ ...d, [p.repo]: { state: "loading" } }))
      backend.updateDiff(p.repo, p.latest).then(
        (diff) => live && setDiffs((d) => ({ ...d, [p.repo]: { state: "ok", diff } })),
        (e: unknown) => live && setDiffs((d) => ({ ...d, [p.repo]: { state: "error", error: errorText(t, e) } })),
      )
    }
    return () => {
      live = false
    }
    // The packs are named by key; t only words an error.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])
  return diffs
}

export function UpdateDialog({ repos, onClose }: { repos?: string[]; onClose: () => void }) {
  const { t, outdated, runTask, reload, skills, openResolve } = useApp()
  const all = outdated?.packs ?? []
  const candidates = all.filter((p) => hasUpdates(p) || p.moved.length > 0)
  const [picked, setPicked] = React.useState<Set<string>>(() => new Set(repos?.length ? repos.filter((r) => candidates.some((c) => c.repo === r)) : candidates.map((c) => c.repo)))
  const [addNew, setAddNew] = React.useState<Record<string, string[]>>({})
  const [busy, setBusy] = React.useState(false)
  const diffs = useUpdateDiffs(candidates)
  // Opened for one source: show it first.
  const shown = repos?.length ? [...candidates.filter((c) => repos.includes(c.repo)), ...candidates.filter((c) => !repos.includes(c.repo))] : candidates
  const edited = [...picked].reduce((n, r) => {
    const d = diffs[r]
    return n + (d?.state === "ok" ? d.diff.skills.filter((s) => s.local).length : 0)
  }, 0)

  const go = async () => {
    setBusy(true)
    const r = await runTask(t("updating"), () => backend.update([...picked], { addNew }))
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(t("updatedToast", { n: r.value.length }))
    const backups = r.value.flatMap((h) => h.backups)
    if (backups.length) toast(t("backupsToast", { n: backups.length }))
    toastMergeOutcome(t, r.value, skills, openResolve)
    onClose()
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="flex max-h-[min(46rem,calc(100svh-4rem))] w-[720px] flex-col gap-0 p-0">
        <DialogHeader className="px-6 pt-[22px] pb-4">
          <DialogTitle>{t("updateTitle")}</DialogTitle>
          <DialogDescription>{t("updateHint")}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto [border-top:var(--tblRow)] px-6 py-4">
          {candidates.length === 0 && <p className="fs-body text-mut">{t("noUpdates")}</p>}
          {shown.map((p) => (
            <PackUpdate
              key={p.repo}
              pack={p}
              diff={diffs[p.repo]}
              open={repos?.includes(p.repo)}
              checked={picked.has(p.repo)}
              onChecked={(v) =>
                setPicked((s) => {
                  const n = new Set(s)
                  if (v) n.add(p.repo)
                  else n.delete(p.repo)
                  return n
                })
              }
              addNew={addNew[p.repo] ?? []}
              onAddNew={(list) => setAddNew((m) => ({ ...m, [p.repo]: list }))}
            />
          ))}
        </div>
        <DialogFooter className="m-0 items-center px-6 py-3.5 [border-top:var(--tblRow)]">
          {edited > 0 && <span className="mr-auto fs-meta text-ink">{t("updateLocalSummary", { n: edited })}</span>}
          <Button variant="outline" size="lg" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button size="lg" onClick={go} disabled={picked.size === 0 || busy}>
            {busy && <RefreshIcon spinning />}
            {t("updateSelected", { n: picked.size })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function PackUpdate({
  pack,
  diff,
  open,
  checked,
  onChecked,
  addNew,
  onAddNew,
}: {
  pack: PackStatus
  diff?: DiffState
  /** Open the first skill's changes (the dialog was opened for this source). */
  open?: boolean
  checked: boolean
  onChecked: (v: boolean) => void
  addNew: string[]
  onAddNew: (list: string[]) => void
}) {
  const { t } = useApp()
  const id = `upd-${pack.repo}`
  // One row per skill: changed and moved upstream reads "有改动 · 路径也变了".
  const rows: [string, MessageKey[]][] = []
  const push = (name: string, key: MessageKey) => {
    const row = rows.find(([n]) => n === name)
    if (row) row[1].push(key === "skillMoved" ? "skillMovedToo" : key)
    else rows.push([name, [key]])
  }
  pack.changed.forEach((n) => push(n, "skillChanged"))
  pack.pending.forEach((n) => push(n, "skillPending"))
  pack.gone.forEach((n) => push(n, "skillGone"))
  pack.moved.forEach((n) => push(n.split(": ")[0], "skillMoved"))
  const skillDiff = (name: string) => (diff?.state === "ok" ? diff.diff.skills.find((s) => s.name === name) : undefined)
  return (
    <div className={cn("rounded-ctl border-line p-3", checked && "border-hue")}>
      <div className="flex items-start gap-2.5">
        <Checkbox id={id} checked={checked} onCheckedChange={(v) => onChecked(!!v)} className="mt-0.5" />
        <label htmlFor={id} className="min-w-0 flex-1">
          <span className="flex items-center gap-2 font-heading fs-name font-semibold text-ink">
            <UpdateDot decorative /> {pack.repo}
          </span>
          <span className="mt-0.5 flex items-center gap-1.5 fs-meta text-mut">
            <Sha id={pack.commit} />
            <ArrowRight className="size-3" aria-hidden />
            <Sha id={pack.latest} />
            <span>· {t("commitsN", { n: pack.commits.length })}</span>
          </span>
        </label>
      </div>
      <ul className="mt-2.5 ml-6.5 space-y-1 fs-body">
        {rows.map(([name, keys], i) => (
          <SkillUpdateRow
            key={name}
            name={name}
            label={keys.map((k) => t(k)).join(" · ")}
            diff={keys.includes("skillGone") ? undefined : diff}
            skill={skillDiff(name)}
            defaultOpen={!!open && i === 0}
          />
        ))}
        {pack.new.map((n) => (
          <li key={"new-" + n} className="flex min-h-7 items-center justify-between gap-3">
            <span className="truncate pl-5 font-medium text-ink">{n}</span>
            <label className="flex shrink-0 items-center gap-1.5 text-mut">
              {t("skillNew")} ·
              <Checkbox checked={addNew.includes(n)} onCheckedChange={(v) => onAddNew(v ? [...addNew, n] : addNew.filter((x) => x !== n))} />
              {t("addNewToo")}
            </label>
          </li>
        ))}
      </ul>
      {diff?.state === "error" && <p className="mt-2 ml-6.5 fs-meta text-mut">{t("diffFailed", { error: diff.error })}</p>}
      {pack.commits.length > 0 && (
        <ul className="mt-2.5 ml-6.5 space-y-0.5 [border-left:1.5px_solid_var(--ln)] pl-3 fs-meta text-mut">
          {pack.commits.map((c) => (
            <li key={c.id} className="truncate">
              <Sha id={c.id} className="mr-1.5" />
              {c.title}
            </li>
          ))}
        </ul>
      )}
      {pack.commitsUnknown && <p className="mt-2.5 ml-6.5 fs-meta text-mut">{t("historyCommitsUnknown")}</p>}
      {pack.moreCommits && <p className="mt-1 ml-6.5 fs-meta text-mut">{t("historyMoreCommits")}</p>}
    </div>
  )
}

/** One skill of a source's update: what happens to it, whether you edited it, and its file changes on demand (also a recorded update's, on the history page). */
export function SkillUpdateRow({ name, label, diff, skill, defaultOpen, tags }: { name: string; label: string; diff?: DiffState; skill?: SkillDiff; defaultOpen: boolean; tags?: React.ReactNode }) {
  const { t } = useApp()
  const [open, setOpen] = React.useState(defaultOpen)
  const loading = diff?.state === "loading"
  const viewable = loading || !!skill
  const head = (
    <>
      <span className="truncate font-medium text-ink">{name}</span>
      {skill?.local && <Tag tone="accent">{t("localEdited")}</Tag>}
      {tags}
      <span className="flex-1" />
      <span className="shrink-0 text-mut">{label}</span>
    </>
  )
  if (!viewable) return <li className="flex min-h-7 items-center gap-2 pl-5">{head}</li>
  return (
    <li>
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger className="-mx-1 flex min-h-7 w-[calc(100%+0.5rem)] items-center gap-2 rounded-ctl px-1 text-left hover:bg-row-hv" aria-label={`${name} · ${open ? t("hideChanges") : t("showChanges")}`}>
          <DisclosureChevron open={open} />
          {head}
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <div className="pt-1.5 pb-3 pl-5">
            {skill ? (
              <SkillDiffView diff={skill} />
            ) : (
              <p className="flex items-center gap-2 fs-meta text-mut" role="status">
                <RefreshIcon spinning /> {t("diffLoading")}
              </p>
            )}
          </div>
        </CollapsiblePanel>
      </Collapsible>
    </li>
  )
}

// Apply ----------------------------------------------------------------------

const ACT: Record<PlanItem["action"], MessageKey> = { link: "actLink", unlink: "actUnlink", relink: "actRelink", blocked: "actBlocked" }

export function ApplyDialog({ onClose }: { onClose: () => void }) {
  const { t, plan, runTask, reload, agents } = useApp()
  const [force, setForce] = React.useState(false)
  const [busy, setBusy] = React.useState(false)
  const blocked = plan.some((p) => p.action === "blocked")
  const label = (n: string) => {
    const a = agents.find((x) => x.name === n)
    return a ? agentLabel(a) : n
  }

  const go = async () => {
    setBusy(true)
    const r = await runTask(t("operating"), () => backend.apply({ force }))
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(t("applied", { linked: r.value.linked, unlinked: r.value.unlinked }))
    toastAbsent(t, agents, r.value.absent)
    onClose()
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="max-h-[calc(100svh-2rem)] min-h-0 w-[600px] overflow-hidden">
        <DialogHeader>
          <DialogTitle>{t("applyTitle")}</DialogTitle>
          <DialogDescription>{t("applyHint")}</DialogDescription>
        </DialogHeader>
        {plan.length === 0 ? (
          <p className="fs-body text-mut">{t("nothingPending")}</p>
        ) : (
          <ul className="min-h-0 max-h-[calc(100svh-13rem)] overflow-y-auto divide-y divide-ln rounded-ctl border-line">
            {plan.map((p) => (
              <li key={p.skill + p.agent} className="grid grid-cols-[6rem_1fr] gap-x-3 px-3 py-2 fs-body">
                <span className={cn("font-medium", p.action === "blocked" && "underline decoration-dotted underline-offset-4")}>{t(ACT[p.action])}</span>
                <span className="min-w-0">
                  <span>
                    {p.skill} → {label(p.agent)}
                  </span>
                  <Mono className="block truncate text-mut" title={tildify(p.path)}>{tildify(p.path)}</Mono>
                  {p.reason && <span className="block fs-meta text-mut">{p.reason}</span>}
                </span>
              </li>
            ))}
          </ul>
        )}
        {blocked && (
          <label className="flex items-start gap-2.5 fs-body">
            <Checkbox checked={force} onCheckedChange={(v) => setForce(!!v)} className="mt-0.5" />
            <span>
              {t("forceLabel")}
              <span className="block fs-meta text-mut">{t("forceHint")}</span>
            </span>
          </label>
        )}
        <DialogFooter>
          <Button variant="outline" size="lg" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button size="lg" onClick={go} disabled={busy || plan.length === 0}>
            {busy && <RefreshIcon spinning />}
            {t("apply")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// Sync -----------------------------------------------------------------------

export function SyncDialog({ onClose }: { onClose: () => void }) {
  const { t, status, syncLibrary } = useApp()
  const [remote, setRemote] = React.useState("")
  const [message, setMessage] = React.useState("")
  const [busy, setBusy] = React.useState(false)
  const needsRemote = !status?.remote

  const go = async () => {
    setBusy(true)
    // Synced, or shelved (its toast leads to resolving): either way the dialog is done.
    const r = await syncLibrary({ remote: needsRemote ? remote.trim() : undefined, message: message.trim() || undefined })
    setBusy(false)
    if (r !== "failed") onClose()
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="w-[500px]">
        <DialogHeader>
          <DialogTitle>{t("syncTitle")}</DialogTitle>
          <DialogDescription>{t("syncHint")}</DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            void go()
          }}
        >
          <Field>
            <FieldLabel htmlFor="sync-remote">{t("remoteLabel")}</FieldLabel>
            {needsRemote ? (
              <Input id="sync-remote" autoFocus value={remote} onChange={(e) => setRemote(e.target.value)} placeholder={t("remotePlaceholder")} className="font-mono fs-body" spellCheck={false} />
            ) : (
              <Mono className="rounded-ctl bg-nav px-2.5 py-1.5">{status?.remote}</Mono>
            )}
            <FieldDescription>{status?.changes ? t("changesN", { n: status.changes }) : t("noChanges")}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="sync-msg">{t("messageLabel")}</FieldLabel>
            <Input id="sync-msg" value={message} onChange={(e) => setMessage(e.target.value)} placeholder="skd sync" />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" size="lg" onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button type="submit" size="lg" disabled={busy || (needsRemote && !remote.trim())}>
              {busy && <RefreshIcon spinning />}
              {busy ? t("syncing") : t("backUp")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// Library: use / clone / init -----------------------------------------------

export function LibraryForm({
  mode,
  onDone,
  onCancel,
  autoFocus = true,
}: {
  mode: "use" | "clone" | "init"
  onDone: () => void
  onCancel?: () => void
  autoFocus?: boolean
}) {
  const { t, runTask, run, reload, status } = useApp()
  const [dir, setDir] = React.useState(mode === "init" ? "~/.skilldots" : mode === "clone" ? "~/.skilldots" : "")
  const [url, setUrl] = React.useState("")
  const [busy, setBusy] = React.useState(false)
  // So the agents can use the library right away: the skilldots skill and the skd command. The skill is on by
  // default for a new library only: an existing one (perhaps someone else's, or one it was taken out of) is
  // changed only when asked.
  const [withSkill, setWithSkill] = React.useState(mode === "init")
  const [withCli, setWithCli] = React.useState(true)
  const [cli, setCli] = React.useState<CliStatus | null>(null)
  React.useEffect(() => {
    backend.cliStatus().then(setCli, () => setCli(null))
  }, [])
  const offerCli = !!cli && cli.canInstall && (cli.state === "missing" || cli.state === "stale")
  const expand = (p: string) => p.trim()

  const go = async () => {
    setBusy(true)
    // The skill and the command are extras: when one fails the library still counts, with a note.
    const notes: string[] = []
    const r = await runTask(t("operating"), async () => {
      const lib =
        mode === "init"
          ? await backend.initLibrary(expand(dir), withSkill)
          : mode === "clone"
            ? await backend.cloneLibrary(url.trim(), expand(dir))
            : await backend.useLibrary(expand(dir))
      if (withSkill && lib.skillError) notes.push(t("selfSkillFailed", { error: lib.skillError }))
      else if (withSkill && !lib.selfSkill) {
        await backend.installSelfSkill().catch((e: unknown) => notes.push(t("selfSkillFailed", { error: errorText(t, e) })))
      }
      return lib
    })
    if (r.ok && withCli && offerCli) {
      // After the library: this may show the system's password prompt.
      try {
        await backend.installCli()
      } catch (e) {
        const msg = errorText(t, e)
        notes.push(msg.includes("cancelled") ? t("cliSkipped") : t("cliFailed", { error: msg }))
      }
    }
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(t("libraryReady"))
    notes.forEach((n) => toast.warning(n))
    onDone()
  }

  const pick = async () => {
    const r = await run(() => backend.pickFolder())
    if (r.ok && r.value) setDir(r.value)
  }

  return (
    <form
      className="grid gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        void go()
      }}
    >
      {mode === "clone" && (
        <Field>
          <FieldLabel htmlFor="lib-url">{t("gitUrl")}</FieldLabel>
          <Input id="lib-url" autoFocus={autoFocus} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="git@github.com:you/agent-skills.git" className="font-mono fs-body" spellCheck={false} />
        </Field>
      )}
      <Field>
        <FieldLabel htmlFor="lib-dir">{mode === "clone" ? t("cloneInto") : t("folder")}</FieldLabel>
        <div className="flex gap-2">
          <Input id="lib-dir" autoFocus={autoFocus && mode !== "clone"} value={dir} onChange={(e) => setDir(e.target.value)} className="font-mono fs-body" spellCheck={false} />
          <Button type="button" variant="outline" size="lg" onClick={pick}>
            {t("choose")}
          </Button>
        </div>
        {mode === "use" && <FieldDescription>{t("switchBody")}</FieldDescription>}
      </Field>
      <div className="grid gap-3">
        <label className="flex items-start gap-2.5 fs-body text-ink">
          <Checkbox checked={withSkill} onCheckedChange={(v) => setWithSkill(!!v)} className="mt-0.5" />
          <span>
            <span className="block">{t("obSkill")}</span>
            <span className="mt-0.5 block fs-meta text-mut">{t(status?.bundledSkill ? "obSkillHint" : "obSkillHintOnline")}</span>
          </span>
        </label>
        {cli?.reason === "move" && cli.state !== "installed" && <p className="fs-meta text-mut">{t("cliMoveHint")}</p>}
        {offerCli && (
          <label className="flex items-start gap-2.5 fs-body text-ink">
            <Checkbox checked={withCli} onCheckedChange={(v) => setWithCli(!!v)} className="mt-0.5" />
            <span>
              <span className="block">{t("obCli")}</span>
              <span className="mt-0.5 block fs-meta text-mut">{t("obCliHint")}</span>
            </span>
          </label>
        )}
      </div>
      <div className="flex justify-end gap-2">
        {onCancel && (
          <Button type="button" variant="outline" size="lg" onClick={onCancel}>
            {t("cancel")}
          </Button>
        )}
        <Button type="submit" size="lg" disabled={busy || !dir.trim() || (mode === "clone" && !url.trim())}>
          {busy && <RefreshIcon spinning />}
          {busy ? (mode === "clone" ? t("cloning") : t("creating")) : t("continue")}
        </Button>
      </div>
    </form>
  )
}

export function LibraryDialog({ mode, onClose }: { mode: "use" | "clone" | "init"; onClose: () => void }) {
  const { t, navigate } = useApp()
  const title = mode === "use" ? t("switchTitle") : mode === "clone" ? t("importTitle") : t("newLibraryTitle")
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="w-[500px]">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <LibraryForm
          mode={mode}
          onCancel={onClose}
          onDone={() => {
            onClose()
            navigate("skills")
          }}
        />
      </DialogContent>
    </Dialog>
  )
}
