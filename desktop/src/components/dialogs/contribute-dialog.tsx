import * as React from "react"
import { toast } from "sonner"
import { GitBranch, GitPullRequestArrow, Upload } from "lucide-react"

import { errorText, useApp } from "@/lib/app-state"
import { backend, type ContributeMode, type ContributeOptions, type Ref, type SkillDiff } from "@/lib/backend"
import { cn } from "@/lib/utils"
import { openExternal } from "@/lib/open-url"
import { RefreshIcon } from "@/components/app/bits"
import { SkillDiffView } from "@/components/app/diff-view"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Textarea } from "@/components/ui/textarea"

type Loaded<T> = { state: "loading" } | { state: "ok"; value: T } | { state: "error"; error: string }

/**
 * 提交给上游: sends your edits of a vendored skill to its source repository, as a PR or a direct push. It shows
 * the target repository and branch, the ways that are open now (and why one is not), the commit message and the
 * diff, and the confirm button says what will happen. Nothing is sent before that button.
 */
export function ContributeDialog({ skillRef, onClose }: { skillRef: Ref; onClose: () => void }) {
  const { t, skills, runTask, reload, openDialog } = useApp()
  const skill = skills.find((s) => s.ref.key === skillRef.key)
  const [opts, setOpts] = React.useState<Loaded<ContributeOptions>>({ state: "loading" })
  const [diff, setDiff] = React.useState<Loaded<SkillDiff>>({ state: "loading" })
  const [mode, setMode] = React.useState<ContributeMode | null>(null)
  const [message, setMessage] = React.useState("")
  const [busy, setBusy] = React.useState(false)

  React.useEffect(() => {
    let live = true
    backend.contributeOptions(skillRef).then(
      (o) => {
        if (!live) return
        setOpts({ state: "ok", value: o })
        setMode(o.canPR ? "pr" : o.canPush ? "push" : null)
        setMessage(o.defaultMessage)
      },
      (e: unknown) => live && setOpts({ state: "error", error: errorText(t, e) }),
    )
    backend.localChanges(skillRef).then(
      (d) => live && setDiff({ state: "ok", value: d }),
      (e: unknown) => live && setDiff({ state: "error", error: errorText(t, e) }),
    )
    return () => {
      live = false
    }
    // One skill per dialog; t only words an error.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [skillRef.key])

  const o = opts.state === "ok" ? opts.value : null
  const can = (m: ContributeMode) => !!o && !o.behind && (m === "pr" ? o.canPR : o.canPush)

  const go = async () => {
    if (!o || !mode || !can(mode)) return
    setBusy(true)
    const r = await runTask(t("contributing"), () => backend.contribute(skillRef, mode, message.trim()))
    setBusy(false)
    if (!r.ok) return
    await reload().catch(() => undefined)
    const out = r.value
    if (out.mode === "pr") toast(t("prOpenedToast", { repo: o.repo, n: out.number }), { action: { label: t("openPR"), onClick: () => openExternal(out.url) } })
    else toast(t("pushedToast", { repo: o.repo, branch: out.branch }), { action: out.url ? { label: t("openCommit"), onClick: () => openExternal(out.url) } : undefined })
    onClose()
  }

  const confirmLabel = !o ? t("contributeUpstream") : mode === "push" ? t("confirmPush", { repo: o.repo, branch: o.branch }) : t("confirmPR", { repo: o.repo })

  return (
    <Dialog open onOpenChange={(v) => !v && onClose()}>
      <DialogContent closeLabel={t("close")} className="flex max-h-[min(46rem,calc(100svh-4rem))] w-[680px] flex-col gap-0 p-0">
        <DialogHeader className="px-6 pt-[22px] pb-4">
          <DialogTitle>{t("contributeTitle", { name: skill?.linkName ?? skillRef.key })}</DialogTitle>
          <DialogDescription>{t("contributeHint")}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 space-y-5 overflow-y-auto [border-top:var(--tblRow)] px-6 py-4">
          {opts.state === "loading" && (
            <p className="flex items-center gap-2 fs-meta text-mut" role="status">
              <RefreshIcon spinning /> {t("contributeLoading")}
            </p>
          )}
          {opts.state === "error" && (
            <p className="rounded-ctl bg-row-hv p-3 fs-meta text-ink [border-left:3px_solid_var(--ac)]" role="alert">
              {opts.error}
            </p>
          )}
          {o && (
            <>
              <div>
                <p className="mb-1 fs-meta font-medium text-ink">{t("targetRepo")}</p>
                <p className="flex flex-wrap items-center gap-x-2 gap-y-0.5 fs-body text-ink">
                  <span className="font-mono">{o.repo}</span>
                  <span className="flex items-center gap-1 fs-meta text-mut">
                    <GitBranch aria-hidden className="size-3.5" />
                    <span className="font-mono">{o.branch}</span>
                  </span>
                </p>
              </div>

              {o.behind ? (
                <div className="rounded-ctl bg-row-hv p-3 fs-meta leading-relaxed text-ink [border-left:3px_solid_var(--ac)]" role="note">
                  <p className="font-semibold">{t("behindTitle")}</p>
                  {o.reason && <p className="mt-0.5">{o.reason}</p>}
                  <Button size="sm" className="mt-2.5" onClick={() => openDialog({ kind: "update", repos: [o.repo] })}>
                    {t("goUpdate")}
                  </Button>
                </div>
              ) : (
                <fieldset>
                  <legend className="mb-1.5 fs-meta font-medium text-ink">{t("contributeMode")}</legend>
                  <RadioGroup value={mode ?? ""} onValueChange={(v) => setMode(v as ContributeMode)} className="grid grid-cols-2 gap-2 max-[640px]:grid-cols-1">
                    {(
                      [
                        ["pr", GitPullRequestArrow, t("modePR"), t("modePRHint", { repo: o.repo })],
                        ["push", Upload, t("modePush"), t("modePushHint", { branch: o.branch })],
                      ] as const
                    ).map(([m, Icon, label, hint]) => {
                      const ok = can(m)
                      return (
                        <label
                          key={m}
                          className={cn(
                            "flex items-start gap-2.5 rounded-ctl border-line p-2.5",
                            ok ? "cursor-pointer has-data-checked:border-hue has-data-checked:bg-row-hv" : "cursor-not-allowed bg-nav",
                          )}
                        >
                          <RadioGroupItem value={m} disabled={!ok} className="mt-0.5" />
                          <span className="min-w-0">
                            <span className={cn("flex items-center gap-1.5 fs-body font-medium", ok ? "text-ink" : "text-mut")}>
                              <Icon aria-hidden className="size-3.5" />
                              {label}
                            </span>
                            {/* An option that is closed says why (the bridge's reason: no push rights, gh missing or logged out). */}
                            <span className="mt-0.5 block fs-meta leading-relaxed text-mut">{ok ? hint : o.reason}</span>
                          </span>
                        </label>
                      )
                    })}
                  </RadioGroup>
                </fieldset>
              )}

              <Field>
                <FieldLabel htmlFor="contribute-msg">{t("commitMessage")}</FieldLabel>
                <Textarea id="contribute-msg" value={message} onChange={(e) => setMessage(e.target.value)} rows={3} disabled={o.behind} className="min-h-[4.5rem] resize-y fs-body" />
              </Field>
            </>
          )}

          <div>
            <p className="mb-1.5 fs-meta font-medium text-ink">{t("changesToSend")}</p>
            {diff.state === "loading" && (
              <p className="flex items-center gap-2 fs-meta text-mut" role="status">
                <RefreshIcon spinning /> {t("diffLoading")}
              </p>
            )}
            {diff.state === "error" && <p className="fs-meta text-mut">{t("diffFailed", { error: diff.error })}</p>}
            {diff.state === "ok" &&
              (diff.value.files.length ? (
                <>
                  {diff.value.base === false && <p className="mb-2 fs-meta leading-relaxed text-mut">{t("myChangesNoBase")}</p>}
                  <SkillDiffView diff={diff.value} note={false} />
                </>
              ) : (
                <p className="fs-meta text-mut">{t("myChangesNone")}</p>
              ))}
          </div>
        </div>
        <DialogFooter className="m-0 items-center px-6 py-3.5 [border-top:var(--tblRow)]">
          <Button variant="outline" size="lg" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button size="lg" onClick={() => void go()} disabled={!o || !mode || !can(mode) || !message.trim() || busy || (diff.state === "ok" && !diff.value.files.length)}>
            {busy ? <RefreshIcon spinning /> : mode === "push" ? <Upload /> : <GitPullRequestArrow />}
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
