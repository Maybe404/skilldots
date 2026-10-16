import * as React from "react"
import { toast } from "sonner"
import { Loader2 } from "lucide-react"

import { useApp, useEnabledAgents } from "@/lib/app-state"
import { backend, type Ref } from "@/lib/backend"
import { useSkillActions } from "@/lib/use-actions"
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
      <legend className="mb-2 text-sm font-medium">{t("location")}</legend>
      <RadioGroup value={value} onValueChange={(v) => onChange(v as "public" | "private")} className="gap-2">
        {(
          [
            ["public", t("locPublic"), t("locPublicHint")],
            ["private", t("locPrivate"), t("locPrivateHint")],
          ] as const
        ).map(([v, label, hint]) => (
          <label key={v} className="flex cursor-default items-start gap-2.5 rounded-lg border p-2.5 has-data-checked:border-foreground/60">
            <RadioGroupItem value={v} className="mt-0.5" />
            <span>
              <span className="block text-[0.85rem] font-medium">{label}</span>
              <span className="block text-[0.78rem] text-muted-foreground">{hint}</span>
            </span>
          </label>
        ))}
      </RadioGroup>
    </fieldset>
  )
}

export function NewSkillDialog({ onClose }: { onClose: () => void }) {
  const { t, run, reload, agents: all, openEditor, select } = useApp()
  const enabled = useEnabledAgents()
  const [name, setName] = React.useState("")
  const [desc, setDesc] = React.useState("")
  const [loc, setLoc] = React.useState<"public" | "private">("public")
  const [targets, setTargets] = React.useState<Set<string>>(() => new Set(all.filter((a) => a.isDefault).map((a) => a.name)))
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
    else select(r.value.ref.key)
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("newTitle")}</DialogTitle>
        </DialogHeader>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (name && !invalid) void create(true)
          }}
        >
          <Field data-invalid={invalid || undefined}>
            <FieldLabel htmlFor="new-name">{t("nameLabel")}</FieldLabel>
            <Input id="new-name" autoFocus value={name} onChange={(e) => setName(e.target.value)} className="font-mono" spellCheck={false} aria-invalid={invalid || undefined} />
            {invalid ? <FieldError>{t("nameInvalid")}</FieldError> : <FieldDescription>{t("nameHint")}</FieldDescription>}
          </Field>
          <Field>
            <FieldLabel htmlFor="new-desc">{t("descLabel")}</FieldLabel>
            <Textarea id="new-desc" value={desc} onChange={(e) => setDesc(e.target.value)} placeholder={t("descPlaceholder")} rows={3} />
            <FieldDescription>{t("descHint")}</FieldDescription>
          </Field>
          <LocationField value={loc} onChange={setLoc} />
          <fieldset>
            <legend className="mb-2 text-sm font-medium">{t("installTo")}</legend>
            <div className="flex flex-wrap gap-x-5 gap-y-2">
              {enabled.map((a) => (
                <label key={a.name} className="flex items-center gap-2 text-[0.85rem]">
                  <Checkbox
                    checked={targets.has(a.name)}
                    onCheckedChange={(v) =>
                      setTargets((s) => {
                        const n = new Set(s)
                        if (v) n.add(a.name)
                        else n.delete(a.name)
                        return n
                      })
                    }
                  />
                  {a.label}
                </label>
              ))}
            </div>
          </fieldset>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button type="button" variant="secondary" disabled={!name || invalid || busy} onClick={() => create(false)}>
              {t("create")}
            </Button>
            <Button type="submit" disabled={!name || invalid || busy}>
              {busy && <Loader2 className="animate-spin motion-reduce:animate-none" />}
              {t("createAndEdit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function ForkDialog({ skillRef, thenEdit, onClose }: { skillRef: Ref; thenEdit?: boolean; onClose: () => void }) {
  const { t, run, reload, skills, openEditor, select } = useApp()
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
    else select(r.value.ref.key)
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
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
            <Input id="fork-name" autoFocus value={name} onChange={(e) => setName(e.target.value)} className="font-mono" spellCheck={false} aria-invalid={invalid || undefined} />
            {invalid ? <FieldError>{t("nameInvalid")}</FieldError> : <FieldDescription>{t("nameHint")}</FieldDescription>}
          </Field>
          <LocationField value={loc} onChange={setLoc} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("cancel")}
            </Button>
            {!thenEdit && (
              <Button type="button" variant="secondary" disabled={!name || invalid || busy} onClick={() => fork(false)}>
                {t("forkAction")}
              </Button>
            )}
            <Button type="submit" disabled={!name || invalid || busy}>
              {busy && <Loader2 className="animate-spin motion-reduce:animate-none" />}
              {t("forkAndEdit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** Remove a vendored skill / unlink one of yours / purge a pack. */
export function ConfirmDialog({ kind, skillRef, repo, onClose }: { kind: "remove" | "unlink" | "purge"; skillRef?: Ref; repo?: string; onClose: () => void }) {
  const { t, run, reload, skills, select } = useApp()
  const { trashToast } = useSkillActions()
  const skill = skillRef ? skills.find((s) => s.ref.key === skillRef.key) : undefined
  const [busy, setBusy] = React.useState(false)
  const n = repo ? skills.filter((s) => s.kind === "vendor" && s.origin === repo).length : 0

  const title = kind === "purge" ? t("purgeTitle", { repo: repo! }) : kind === "remove" ? t("removeTitle", { name: skill?.linkName ?? "" }) : t("unlinkTitle", { name: skill?.linkName ?? "" })
  const body = kind === "purge" ? t("purgeBody", { n }) : kind === "remove" ? t("removeBody") : t("unlinkBody")
  const action = kind === "purge" ? t("removeSource") : kind === "remove" ? t("moveToTrash") : t("unlinkAll")

  const go = async () => {
    setBusy(true)
    const r = await run(() =>
      kind === "purge" ? backend.purge(repo!) : kind === "remove" ? backend.remove(skill!.ref) : backend.disable(skill!.ref, []),
    )
    setBusy(false)
    if (!r.ok) return
    if (kind !== "unlink") select(null)
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
    }
    else trashToast(kind === "purge" ? repo! : skill!.linkName)
    onClose()
  }

  return (
    <AlertDialog open onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{body}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={go} disabled={busy}>
            {action}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
