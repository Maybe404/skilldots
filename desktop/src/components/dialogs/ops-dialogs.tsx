import * as React from "react"
import { toast } from "sonner"
import { ArrowRight, Loader2 } from "lucide-react"

import { cn } from "@/lib/utils"
import { hasUpdates, useApp } from "@/lib/app-state"
import { backend, type PackStatus, type PlanItem } from "@/lib/backend"
import type { MessageKey } from "@/lib/i18n"
import { Mono, UpdateDot } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"

// Update ---------------------------------------------------------------------

export function UpdateDialog({ repos, onClose }: { repos?: string[]; onClose: () => void }) {
  const { t, outdated, run, reload } = useApp()
  const all = outdated?.packs ?? []
  const candidates = all.filter((p) => hasUpdates(p))
  const movedOnly = all.filter((p) => !hasUpdates(p) && p.moved.length > 0)
  const [picked, setPicked] = React.useState<Set<string>>(() => new Set(repos?.length ? repos.filter((r) => candidates.some((c) => c.repo === r)) : candidates.map((c) => c.repo)))
  const [addNew, setAddNew] = React.useState<Record<string, string[]>>({})
  const [busy, setBusy] = React.useState(false)

  const go = async () => {
    setBusy(true)
    const r = await run(() => backend.update([...picked], { addNew }))
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(t("updatedToast", { n: r.value.length }))
    const backups = r.value.flatMap((h) => h.backups)
    if (backups.length) toast(t("backupsToast", { n: backups.length }))
    onClose()
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="flex max-h-[min(42rem,calc(100svh-4rem))] flex-col gap-0 p-0 sm:max-w-xl">
        <DialogHeader className="px-5 pt-5 pb-4">
          <DialogTitle>{t("updateTitle")}</DialogTitle>
          <DialogDescription>{t("updateHint")}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto border-t px-5 py-4">
          {candidates.length === 0 && <p className="text-sm text-muted-foreground">{t("noUpdates")}</p>}
          {candidates.map((p) => (
            <PackUpdate
              key={p.repo}
              pack={p}
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
          {movedOnly.length > 0 && (
            <div className="rounded-lg border border-dashed p-3 text-[0.8rem] text-muted-foreground">
              <p>{t("moveOnlyNote")}</p>
              <ul className="mt-1.5 space-y-0.5">
                {movedOnly.map((p) => (
                  <li key={p.repo}>
                    <span className="text-foreground">{p.repo}</span> · {p.moved.join(", ")}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
        <DialogFooter className="m-0 rounded-b-xl px-5 py-3">
          <Button variant="outline" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button onClick={go} disabled={picked.size === 0 || busy}>
            {busy && <Loader2 className="animate-spin motion-reduce:animate-none" />}
            {t("updateSelected", { n: picked.size })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function PackUpdate({
  pack,
  checked,
  onChecked,
  addNew,
  onAddNew,
}: {
  pack: PackStatus
  checked: boolean
  onChecked: (v: boolean) => void
  addNew: string[]
  onAddNew: (list: string[]) => void
}) {
  const { t } = useApp()
  const id = `upd-${pack.repo}`
  const rows: [string, MessageKey][] = [
    ...pack.changed.map((n) => [n, "skillChanged"] as [string, MessageKey]),
    ...pack.pending.map((n) => [n, "skillPending"] as [string, MessageKey]),
    ...pack.gone.map((n) => [n, "skillGone"] as [string, MessageKey]),
    ...pack.moved.map((n) => [n, "skillMoved"] as [string, MessageKey]),
  ]
  return (
    <div className={cn("rounded-lg border p-3", checked && "border-foreground/40")}>
      <div className="flex items-start gap-2.5">
        <Checkbox id={id} checked={checked} onCheckedChange={(v) => onChecked(!!v)} className="mt-0.5" />
        <label htmlFor={id} className="min-w-0 flex-1">
          <span className="flex items-center gap-2 text-[0.9rem] font-medium">
            <UpdateDot decorative /> {pack.repo}
          </span>
          <span className="mt-0.5 flex items-center gap-1.5 text-[0.78rem] text-muted-foreground">
            <Mono>{pack.commit}</Mono>
            <ArrowRight className="size-3" aria-hidden />
            <Mono>{pack.latest}</Mono>
            <span>· {t("commitsN", { n: pack.commits.length })}</span>
          </span>
        </label>
      </div>
      <ul className="mt-2.5 ml-6.5 space-y-1 text-[0.82rem]">
        {rows.map(([name, label]) => (
          <li key={name + label} className="flex items-baseline justify-between gap-3">
            <Mono className="truncate">{name}</Mono>
            <span className="shrink-0 text-muted-foreground">{t(label)}</span>
          </li>
        ))}
        {pack.new.map((n) => (
          <li key={"new-" + n} className="flex items-center justify-between gap-3">
            <Mono className="truncate">{n}</Mono>
            <label className="flex shrink-0 items-center gap-1.5 text-muted-foreground">
              {t("skillNew")} ·
              <Checkbox checked={addNew.includes(n)} onCheckedChange={(v) => onAddNew(v ? [...addNew, n] : addNew.filter((x) => x !== n))} />
              {t("addNewToo")}
            </label>
          </li>
        ))}
      </ul>
      {pack.commits.length > 0 && (
        <ul className="mt-2.5 ml-6.5 space-y-0.5 border-l pl-3 text-[0.78rem] text-muted-foreground">
          {pack.commits.map((c) => (
            <li key={c.id} className="truncate">
              <Mono className="mr-1.5">{c.id}</Mono>
              {c.title}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// Apply ----------------------------------------------------------------------

const ACT: Record<PlanItem["action"], MessageKey> = { link: "actLink", unlink: "actUnlink", relink: "actRelink", blocked: "actBlocked" }

export function ApplyDialog({ onClose }: { onClose: () => void }) {
  const { t, plan, run, reload, agents } = useApp()
  const [force, setForce] = React.useState(false)
  const [busy, setBusy] = React.useState(false)
  const blocked = plan.some((p) => p.action === "blocked")
  const label = (n: string) => agents.find((a) => a.name === n)?.label ?? n

  const go = async () => {
    setBusy(true)
    const r = await run(() => backend.apply({ force }))
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(t("applied", { linked: r.value.linked, unlinked: r.value.unlinked }))
    onClose()
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("applyTitle")}</DialogTitle>
          <DialogDescription>{t("applyHint")}</DialogDescription>
        </DialogHeader>
        {plan.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("nothingPending")}</p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {plan.map((p) => (
              <li key={p.skill + p.agent} className="grid grid-cols-[6rem_1fr] gap-x-3 px-3 py-2 text-[0.82rem]">
                <span className={cn("font-medium", p.action === "blocked" && "underline decoration-dotted underline-offset-4")}>{t(ACT[p.action])}</span>
                <span className="min-w-0">
                  <span>
                    {p.skill} → {label(p.agent)}
                  </span>
                  <Mono className="block truncate text-muted-foreground">{p.path}</Mono>
                  {p.reason && <span className="block text-[0.76rem] text-muted-foreground">{p.reason}</span>}
                </span>
              </li>
            ))}
          </ul>
        )}
        {blocked && (
          <label className="flex items-start gap-2.5 text-[0.85rem]">
            <Checkbox checked={force} onCheckedChange={(v) => setForce(!!v)} className="mt-0.5" />
            <span>
              {t("forceLabel")}
              <span className="block text-[0.78rem] text-muted-foreground">{t("forceHint")}</span>
            </span>
          </label>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button onClick={go} disabled={busy || plan.length === 0}>
            {busy && <Loader2 className="animate-spin motion-reduce:animate-none" />}
            {t("apply")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// Sync -----------------------------------------------------------------------

export function SyncDialog({ onClose }: { onClose: () => void }) {
  const { t, status, run, reload } = useApp()
  const [remote, setRemote] = React.useState("")
  const [message, setMessage] = React.useState("")
  const [busy, setBusy] = React.useState(false)
  const needsRemote = !status?.remote

  const go = async () => {
    setBusy(true)
    const r = await run(() => backend.sync({ remote: needsRemote ? remote.trim() : undefined, message: message.trim() || undefined }))
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(r.value.committed ? t("syncedToast", { remote: r.value.remote }) : t("nothingToSync"))
    onClose()
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="sm:max-w-lg">
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
              <Input id="sync-remote" autoFocus value={remote} onChange={(e) => setRemote(e.target.value)} placeholder={t("remotePlaceholder")} className="font-mono text-[0.82rem]" spellCheck={false} />
            ) : (
              <Mono className="rounded-md bg-muted px-2.5 py-1.5">{status?.remote}</Mono>
            )}
            <FieldDescription>{status?.changes ? t("changesN", { n: status.changes }) : t("noChanges")}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="sync-msg">{t("messageLabel")}</FieldLabel>
            <Input id="sync-msg" value={message} onChange={(e) => setMessage(e.target.value)} placeholder="skd sync" />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button type="submit" disabled={busy || (needsRemote && !remote.trim())}>
              {busy && <Loader2 className="animate-spin motion-reduce:animate-none" />}
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
  const { t, run, reload } = useApp()
  const [dir, setDir] = React.useState(mode === "init" ? "~/.skilldots" : mode === "clone" ? "~/.skilldots" : "")
  const [url, setUrl] = React.useState("")
  const [busy, setBusy] = React.useState(false)
  const expand = (p: string) => p.replace(/^~/, "/Users/maybe")

  const go = async () => {
    setBusy(true)
    const r = await run(() =>
      mode === "init" ? backend.initLibrary(expand(dir)) : mode === "clone" ? backend.cloneLibrary(url.trim(), expand(dir)) : backend.useLibrary(expand(dir)),
    )
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(t("libraryReady"), { description: mode === "init" ? t("skilldotsInstalled") : undefined })
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
          <Input id="lib-url" autoFocus={autoFocus} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="git@github.com:you/agent-skills.git" className="font-mono text-[0.82rem]" spellCheck={false} />
        </Field>
      )}
      <Field>
        <FieldLabel htmlFor="lib-dir">{mode === "clone" ? t("cloneInto") : t("folder")}</FieldLabel>
        <div className="flex gap-2">
          <Input id="lib-dir" autoFocus={autoFocus && mode !== "clone"} value={dir} onChange={(e) => setDir(e.target.value)} className="font-mono text-[0.82rem]" spellCheck={false} />
          <Button type="button" variant="outline" onClick={pick}>
            {t("choose")}
          </Button>
        </div>
        {mode === "use" && <FieldDescription>{t("switchBody")}</FieldDescription>}
      </Field>
      <div className="flex justify-end gap-2">
        {onCancel && (
          <Button type="button" variant="outline" onClick={onCancel}>
            {t("cancel")}
          </Button>
        )}
        <Button type="submit" disabled={busy || !dir.trim() || (mode === "clone" && !url.trim())}>
          {busy && <Loader2 className="animate-spin motion-reduce:animate-none" />}
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
      <DialogContent closeLabel={t("close")} className="sm:max-w-lg">
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

// Empty trash: asks twice ----------------------------------------------------

export function EmptyTrashDialog({ onClose }: { onClose: () => void }) {
  const { t, trashCount, run, reload } = useApp()
  const [step, setStep] = React.useState<1 | 2>(1)
  const [busy, setBusy] = React.useState(false)

  const go = async () => {
    setBusy(true)
    const r = await run(() => backend.emptyTrash())
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(t("trashEmptied", { n: r.value }))
    onClose()
  }

  return (
    <>
      <AlertDialog open={step === 1} onOpenChange={(o) => !o && step === 1 && onClose()}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("emptyTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("emptyBody", { n: trashCount })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel autoFocus>{t("cancel")}</AlertDialogCancel>
            <Button variant="outline" onClick={() => setStep(2)}>
              {t("continue")}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={step === 2} onOpenChange={(o) => !o && onClose()}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("emptyConfirm2Title")}</AlertDialogTitle>
            <AlertDialogDescription>{t("emptyConfirm2Body", { n: trashCount })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel autoFocus>{t("cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={go} disabled={busy}>
              {t("emptyConfirm2Action")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
