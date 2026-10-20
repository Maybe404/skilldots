import * as React from "react"
import { toast } from "sonner"

import { cn } from "@/lib/utils"
import { useApp, useEnabledAgents } from "@/lib/app-state"
import { agentLabel } from "@/lib/agents"
import { backend, type Ref } from "@/lib/backend"
import { useSkillActions } from "@/lib/use-actions"
import { Info } from "lucide-react"
import { Hint, RefreshIcon } from "@/components/app/bits"
import { Collapsible, CollapsiblePanel, CollapsibleTrigger, DisclosureChevron } from "@/components/ui/collapsible"
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
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Textarea } from "@/components/ui/textarea"

const NAME = /^[A-Za-z0-9_-][A-Za-z0-9._-]*$/

function LocationField({ value, onChange }: { value: "public" | "private"; onChange: (v: "public" | "private") => void }) {
  const { t } = useApp()
  return (
    <fieldset>
      <legend className="mb-1.5 fs-meta font-medium text-ink">{t("location")}</legend>
      <RadioGroup value={value} onValueChange={(v) => onChange(v as "public" | "private")} className="grid grid-cols-2 gap-2">
        {(
          [
            ["public", t("locPublic"), t("locPublicHint")],
            ["private", t("locPrivate"), t("locPrivateHint")],
          ] as const
        ).map(([v, label, hint]) => (
          <label key={v} className="flex cursor-pointer items-start gap-2.5 rounded-ctl border-line p-2.5 has-data-checked:border-hue has-data-checked:bg-row-hv">
            <RadioGroupItem value={v} className="mt-0.5" />
            <span className="min-w-0">
              <span className="block fs-body font-medium text-ink">{label}</span>
              <span className="block fs-meta text-mut">{hint}</span>
            </span>
          </label>
        ))}
      </RadioGroup>
    </fieldset>
  )
}

/**
 * Agent checkboxes for "install to" (new skill, add). The agents found on
 * this Mac come first; the rest fold away (Animate UI collapsible). Which
 * agents count as "here" is fixed when the box opens, so unticking one
 * does not move it.
 */
export function AgentTargets({ value, onChange, className, label }: { value: Set<string>; onChange: (v: Set<string>) => void; className?: string; label?: string }) {
  const { t } = useApp()
  const enabled = useEnabledAgents()
  const [shown] = React.useState(() => new Set(enabled.filter((a) => a.detected || value.has(a.name)).map((a) => a.name)))
  const [showOthers, setShowOthers] = React.useState(false)
  const here = enabled.filter((a) => shown.has(a.name))
  const others = enabled.filter((a) => !shown.has(a.name))
  const box = (a: (typeof enabled)[number]) => (
    <label key={a.name} className="flex items-center gap-2 fs-body text-ink">
      <Checkbox
        checked={value.has(a.name)}
        onCheckedChange={(v) => {
          const n = new Set(value)
          if (v) n.add(a.name)
          else n.delete(a.name)
          onChange(n)
        }}
      />
      {agentLabel(a)}
      {a.name === "agents" && (
        <Hint label={t("sharedDirFull")} wide>
          <Info aria-label={t("sharedDirFull")} className="size-3.5 text-mut" strokeWidth={1.8} />
        </Hint>
      )}
    </label>
  )
  return (
    <fieldset className={className}>
      <legend className="mb-1.5 fs-meta font-medium text-ink">{label ?? t("installTo")}</legend>
      <div className="flex flex-wrap gap-x-5 gap-y-2">{here.map(box)}</div>
      {others.length > 0 && (
        <Collapsible open={showOthers} onOpenChange={setShowOthers} className="mt-2">
          <CollapsibleTrigger className="flex h-6 items-center gap-1.5 rounded-seg fs-meta text-mut hover:text-ink">
            <DisclosureChevron open={showOthers} />
            {t("otherAgentsHere", { n: others.length })}
          </CollapsibleTrigger>
          <CollapsiblePanel>
            <div className="flex flex-wrap gap-x-5 gap-y-2 pt-1.5 pb-0.5">{others.map(box)}</div>
          </CollapsiblePanel>
        </Collapsible>
      )}
    </fieldset>
  )
}

export function NewSkillDialog({ onClose }: { onClose: () => void }) {
  const { t, run, reload, agents: all, openEditor, select, navigate } = useApp()
  const [name, setName] = React.useState("")
  const [desc, setDesc] = React.useState("")
  const [loc, setLoc] = React.useState<"public" | "private">("public")
  const [targets, setTargets] = React.useState<Set<string>>(() => new Set(all.filter((a) => a.isDefault && a.enabled).map((a) => a.name)))
  const [busy, setBusy] = React.useState(false)
  const invalid = name !== "" && !NAME.test(name)

  const create = async (edit: boolean) => {
    setBusy(true)
    const r = await run(() => backend.newSkill({ name, description: desc, private: loc === "private", agents: [...targets] }))
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(t("created", { name: r.value.name }))
    onClose()
    if (edit) openEditor(r.value.ref)
    else {
      navigate("skills")
      select(r.value.ref.key)
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="flex w-[520px] flex-col overflow-hidden">
        <DialogHeader>
          <DialogTitle>{t("newTitle")}</DialogTitle>
          <DialogDescription>{t("newHint")}</DialogDescription>
        </DialogHeader>
        <form
          className="flex min-h-0 flex-1 flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (name && !invalid) void create(true)
          }}
        >
          {/* The fields scroll when the window is short; title and buttons stay in view. */}
          <div className="-mx-1 grid min-h-0 gap-4 overflow-y-auto px-1 py-1">
            <Field data-invalid={invalid || undefined}>
              <FieldLabel htmlFor="new-name">{t("nameLabel")}</FieldLabel>
              <Input id="new-name" autoFocus value={name} onChange={(e) => setName(e.target.value)} spellCheck={false} aria-invalid={invalid || undefined} />
              {invalid ? <FieldError>{t("nameInvalid")}</FieldError> : <FieldDescription>{t("nameHint")}</FieldDescription>}
            </Field>
            <Field>
              <FieldLabel htmlFor="new-desc">{t("descLabel")}</FieldLabel>
              <Textarea id="new-desc" value={desc} onChange={(e) => setDesc(e.target.value)} placeholder={t("descPlaceholder")} rows={3} />
              <FieldDescription>{t("descHint")}</FieldDescription>
            </Field>
            <LocationField value={loc} onChange={setLoc} />
            <AgentTargets value={targets} onChange={setTargets} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" size="lg" onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button type="button" variant="secondary" size="lg" disabled={!name || invalid || busy} onClick={() => create(false)}>
              {t("create")}
            </Button>
            <Button type="submit" variant="ink" size="lg" disabled={!name || invalid || busy}>
              {busy && <RefreshIcon spinning />}
              {t("createAndEdit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function ForkDialog({ skillRef, thenEdit, onClose }: { skillRef: Ref; thenEdit?: boolean; onClose: () => void }) {
  const { t, run, reload, skills, openEditor, select, navigate } = useApp()
  const skill = skills.find((s) => s.ref.key === skillRef.key)
  const [name, setName] = React.useState(skill?.name ?? "")
  const [loc, setLoc] = React.useState<"public" | "private">("public")
  const [busy, setBusy] = React.useState(false)
  if (!skill) return null
  const invalid = name !== "" && !NAME.test(name)

  const fork = async (edit: boolean) => {
    setBusy(true)
    const r = await run(() => backend.fork(skill.ref, { private: loc === "private", name }))
    setBusy(false)
    if (!r.ok) return
    await reload()
    toast(t("forked", { name: r.value.name }))
    onClose()
    if (edit) openEditor(r.value.ref)
    else {
      navigate("skills")
      select(r.value.ref.key)
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t("close")} className="w-[520px]">
        <DialogHeader>
          <DialogTitle>{t("forkTitle", { name: skill.linkName })}</DialogTitle>
          <DialogDescription>{t("forkBody")}</DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (name && !invalid) void fork(true)
          }}
        >
          <Field data-invalid={invalid || undefined}>
            <FieldLabel htmlFor="fork-name">{t("nameLabel")}</FieldLabel>
            <Input id="fork-name" autoFocus value={name} onChange={(e) => setName(e.target.value)} spellCheck={false} aria-invalid={invalid || undefined} />
            {invalid ? <FieldError>{t("nameInvalid")}</FieldError> : <FieldDescription>{t("nameHint")}</FieldDescription>}
          </Field>
          <LocationField value={loc} onChange={setLoc} />
          <DialogFooter>
            <Button type="button" variant="outline" size="lg" onClick={onClose}>
              {t("cancel")}
            </Button>
            {!thenEdit && (
              <Button type="button" variant="secondary" size="lg" disabled={!name || invalid || busy} onClick={() => fork(false)}>
                {t("forkAction")}
              </Button>
            )}
            <Button type="submit" variant="ink" size="lg" disabled={!name || invalid || busy}>
              {busy && <RefreshIcon spinning />}
              {t("forkAndEdit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/**
 * The design's confirmation box (title, one paragraph, 取消 + an ink button).
 * The action runs inside the dialog so errors keep it open; Base UI's alert
 * dialog keeps focus inside and returns it afterwards.
 */
export function ConfirmBox({
  title,
  body,
  action,
  run,
  onClose,
}: {
  title: string
  body: React.ReactNode
  action: string
  run: () => Promise<boolean | void> | boolean | void
  onClose: () => void
}) {
  const { t } = useApp()
  const [busy, setBusy] = React.useState(false)
  const go = async () => {
    setBusy(true)
    const keep = (await run()) === false
    setBusy(false)
    if (!keep) onClose()
  }
  return (
    <AlertDialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{body}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy}>{t("cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={() => void go()} disabled={busy} className={cn(busy && "cursor-progress")}>
            {busy && <RefreshIcon spinning />}
            {action}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

/** Move a skill to the trash / unlink an app's skill / remove a whole source. */
export function ConfirmDialog({ kind, skillRef, repo, onClose }: { kind: "remove" | "unlink" | "purge"; skillRef?: Ref; repo?: string; onClose: () => void }) {
  const { t, run, reload, skills, select, closeDetail } = useApp()
  const { trashToast } = useSkillActions()
  const skill = skillRef ? skills.find((s) => s.ref.key === skillRef.key) : undefined
  const used = repo ? skills.filter((s) => s.kind === "vendor" && s.origin === repo) : []

  const title = kind === "purge" ? t("purgeTitle", { repo: repo! }) : kind === "remove" ? t("removeTitle", { name: skill?.linkName ?? "" }) : t("unlinkTitle", { name: skill?.linkName ?? "" })
  const body =
    kind === "purge"
      ? t("purgeBody", { n: used.length, names: used.map((s) => s.linkName).join(t("listSep")) || "—" })
      : kind === "remove"
        ? t("removeBody")
        : t("unlinkBody")
  const action = kind === "purge" ? t("purgeAction") : kind === "remove" ? t("moveToTrash") : t("unlinkAll")

  const go = async () => {
    const r = await run(() => (kind === "purge" ? backend.purge(repo!) : kind === "remove" ? backend.remove(skill!.ref) : backend.disable(skill!.ref, [])))
    if (!r.ok) return false
    if (kind !== "unlink") {
      select(null)
      closeDetail()
    }
    await reload()
    if (kind === "unlink") {
      const prev = [...skill!.targets]
      toast(t("unlinkedAll", { skill: skill!.linkName }), {
        action: {
          label: t("undo"),
          onClick: async () => {
            await run(() => backend.setTargets(skill!.ref, prev))
            await reload()
          },
        },
      })
    } else trashToast(kind === "purge" ? repo! : skill!.linkName)
  }

  return <ConfirmBox title={title} body={body} action={action} run={go} onClose={onClose} />
}
