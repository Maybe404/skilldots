import * as React from "react"
import { toast } from "sonner"
import { ArrowDownToLine, GitBranch, GitPullRequestArrow, Upload } from "lucide-react"

import { errorText, useApp } from "@/lib/app-state"
import { backend, type ContributeMode, type ContributeOptions, type Ref, type SkillDiff } from "@/lib/backend"
import { cn } from "@/lib/utils"
import { openExternal } from "@/lib/open-url"
import { Mono, RefreshIcon, Tag } from "@/components/app/bits"
import { SkillDiffView } from "@/components/app/diff-view"
import { Button } from "@/components/ui/button"
import { Collapsible, CollapsiblePanel, CollapsibleTrigger, DisclosureChevron } from "@/components/ui/collapsible"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Textarea } from "@/components/ui/textarea"

type Loaded<T> = { state: "loading" } | { state: "ok"; value: T } | { state: "error"; error: string }

/**
 * 提交给上游: sends your edits of a vendored skill (skillRef), or of every skill of a source you changed or made
 * there (repo, 推送 N 项), to the source repository, as a PR or a direct push. It shows the target repository and
 * branch, the ways that are open now (and why one is not), the commit message and each skill's diff, and the
 * confirm button says what will happen. Nothing is sent before that button. In your own repository (own) a push
 * is the default and a PR the secondary way; being behind does not block there, as sending pulls first (pull).
 */
export function ContributeDialog({ skillRef, repo, onClose }: { skillRef?: Ref; repo?: string; onClose: () => void }) {
  const { t, skills, packs, runTask, reload, openDialog, check, showSource } = useApp()
  const skill = skillRef ? skills.find((s) => s.ref.key === skillRef.key) : undefined
  const target = repo ?? skill?.origin ?? ""
  const [opts, setOpts] = React.useState<Loaded<ContributeOptions>>({ state: "loading" })
  const [diffs, setDiffs] = React.useState<Record<string, Loaded<SkillDiff>>>({})
  const [mode, setMode] = React.useState<ContributeMode | null>(null)
  const [message, setMessage] = React.useState("")
  const [busy, setBusy] = React.useState(false)

  React.useEffect(() => {
    let live = true
    const started = new Set<string>()
    // Each skill's changes, by link name (what the options list).
    const loadDiff = (name: string, ref: Ref | undefined) => {
      if (started.has(name)) return
      started.add(name)
      if (!ref) return setDiffs((d) => ({ ...d, [name]: { state: "error", error: name } }))
      setDiffs((d) => ({ ...d, [name]: { state: "loading" } }))
      backend.localChanges(ref).then(
        (v) => live && setDiffs((d) => ({ ...d, [name]: { state: "ok", value: v } })),
        (e: unknown) => live && setDiffs((d) => ({ ...d, [name]: { state: "error", error: errorText(t, e) } })),
      )
    }
    // One skill: its diff loads alongside the options.
    if (skillRef) loadDiff(skill?.linkName ?? skillRef.key, skillRef)
    ;(skillRef ? backend.contributeOptions(skillRef) : backend.contributePackOptions(target)).then(
      (o) => {
        if (!live) return
        setOpts({ state: "ok", value: o })
        setMode(o.own ? (o.canPush ? "push" : o.canPR ? "pr" : null) : o.canPR ? "pr" : o.canPush ? "push" : null)
        setMessage(o.defaultMessage)
        if (!skillRef) for (const name of o.skills) loadDiff(name, skills.find((s) => s.kind === "vendor" && s.origin === o.repo && s.linkName === name)?.ref)
      },
      (e: unknown) => live && setOpts({ state: "error", error: errorText(t, e) }),
    )
    return () => {
      live = false
    }
    // One skill or source per dialog; t only words an error, and the skills are read once to find the refs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [skillRef?.key, target])

  const o = opts.state === "ok" ? opts.value : null
  const own = o ? o.own : !!(skill?.own ?? packs.find((p) => p.repo === target)?.own)
  // Behind upstream blocks, unless sending pulls first (your own repository).
  const blocked = !!o && o.behind && !o.pull
  const can = (m: ContributeMode) => !!o && !blocked && (m === "pr" ? o.canPR : o.canPush)
  const names = o?.skills.length ? o.skills : skillRef ? [skill?.linkName ?? skillRef.key] : []
  const drafts = new Set(o?.drafts ?? [])
  // Every diff read and empty: nothing would be sent.
  const nothing =
    names.length > 0 &&
    names.every((n) => {
      const d = diffs[n]
      return !drafts.has(n) && d?.state === "ok" && d.value.files.length === 0
    })

  const go = async () => {
    if (!o || !mode || !can(mode)) return
    setBusy(true)
    const msg = message.trim()
    const r = await runTask(t(own && mode === "push" ? "pushing" : "contributing"), () => (skillRef ? backend.contribute(skillRef, mode, msg) : backend.contributePack(o.repo, mode, msg)))
    setBusy(false)
    if (!r.ok) return
    await reload().catch(() => undefined)
    const out = r.value
    const n = out.skills?.length || names.length
    if (out.mode === "pr") toast(t("prOpenedToast", { repo: o.repo, n: out.number }), { action: { label: t("openPR"), onClick: () => openExternal(out.url) } })
    else {
      const title = n > 1 ? t("pushedNToast", { n, repo: o.repo, branch: out.branch }) : t("pushedToast", { repo: o.repo, branch: out.branch })
      if (out.pullBackError)
        // The push happened; updating the source brings it back.
        toast.warning(title, {
          description: t("pullBackFailed", { error: out.pullBackError }),
          duration: 10000,
          action: {
            label: t("goUpdate"),
            onClick: () => {
              showSource(o.repo)
              void check([o.repo])
            },
          },
        })
      else {
        const notes = [out.pulled && t("pulledFirstNote"), out.pulledBack && t("pulledBackNote")].filter(Boolean).join(t("mergeSep"))
        toast(title, { description: notes || undefined, action: out.url ? { label: t("openCommit"), onClick: () => openExternal(out.url) } : undefined })
      }
    }
    onClose()
  }

  const title = skillRef
    ? own
      ? t("pushTitle", { name: skill?.linkName ?? skillRef.key, repo: target })
      : t("contributeTitle", { name: skill?.linkName ?? skillRef.key })
    : own
      ? t("pushToRepo", { repo: target })
      : t("contributePackTitle", { repo: target })
  const hint = skillRef ? t(own ? "pushHint" : "contributeHint") : t(own ? "pushPackHint" : "contributePackHint")
  const confirmLabel = !o
    ? own
      ? t("pushToRepo", { repo: target })
      : t("contributeUpstream")
    : mode === "push"
      ? names.length > 1
        ? t("confirmPushN", { n: names.length, repo: o.repo, branch: o.branch })
        : t("confirmPush", { repo: o.repo, branch: o.branch })
      : t("confirmPR", { repo: o.repo })

  return (
    <Dialog open onOpenChange={(v) => !v && onClose()}>
      <DialogContent closeLabel={t("close")} className="flex max-h-[min(46rem,calc(100svh-4rem))] w-[680px] flex-col gap-0 p-0">
        <DialogHeader className="px-6 pt-[22px] pb-4">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{hint}</DialogDescription>
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
                {o.pull && (
                  <p className="mt-1.5 flex items-center gap-1.5 fs-meta text-mut" role="note">
                    <ArrowDownToLine aria-hidden className="size-3.5" />
                    {t("pullFirst")}
                  </p>
                )}
              </div>

              {blocked ? (
                <div className="rounded-ctl bg-row-hv p-3 fs-meta leading-relaxed text-ink [border-left:3px_solid_var(--ac)]" role="note">
                  <p className="font-semibold">{t("behindTitle")}</p>
                  {o.reason && <p className="mt-0.5">{o.reason}</p>}
                  <Button size="sm" className="mt-2.5" onClick={() => openDialog({ kind: "update", repos: [o.repo] })}>
                    {t("goUpdate")}
                  </Button>
                </div>
              ) : own && o.canPush ? (
                // Your own repository: a push, with a PR tucked away as the other way.
                (o.canPR || mode === "pr") && (
                  <div className="fs-meta text-mut">
                    {mode === "pr" && (
                      <p className="mb-1.5 flex items-center gap-1.5 text-ink">
                        <GitPullRequestArrow aria-hidden className="size-3.5" />
                        {t("modePRHint", { repo: o.repo })}
                      </p>
                    )}
                    <Button variant="quiet" size="inline" className="fs-meta" onClick={() => setMode(mode === "pr" ? "push" : "pr")}>
                      {mode === "pr" ? t("switchToPush") : t("switchToPR")}
                    </Button>
                  </div>
                )
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
                <Textarea id="contribute-msg" value={message} onChange={(e) => setMessage(e.target.value)} rows={3} disabled={blocked} className="min-h-[4.5rem] resize-y fs-body" />
              </Field>
            </>
          )}

          {names.length > 0 && (
            <div>
              <p className="mb-1.5 fs-meta font-medium text-ink">{t("changesToSend")}</p>
              <div className="[border-top:var(--tblRow)]">
                {names.map((n) => (
                  <SkillChanges key={n} name={n} draft={drafts.has(n)} path={o?.paths?.[n]} diff={diffs[n]} defaultOpen={names.length === 1} />
                ))}
              </div>
            </div>
          )}
        </div>
        <DialogFooter className="m-0 items-center px-6 py-3.5 [border-top:var(--tblRow)]">
          <Button variant="outline" size="lg" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button size="lg" onClick={() => void go()} disabled={!o || !mode || !can(mode) || !message.trim() || busy || nothing}>
            {busy ? <RefreshIcon spinning /> : mode === "push" ? <Upload /> : <GitPullRequestArrow />}
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** One skill being sent: its name (新建 and its place in the repository for a draft), and its diff folded until asked for. */
function SkillChanges({ name, draft, path, diff, defaultOpen }: { name: string; draft: boolean; path?: string; diff?: Loaded<SkillDiff>; defaultOpen: boolean }) {
  const { t } = useApp()
  const [open, setOpen] = React.useState(defaultOpen)
  const files = diff?.state === "ok" ? diff.value.files : []
  const added = files.reduce((n, f) => n + f.added, 0)
  const removed = files.reduce((n, f) => n + f.removed, 0)
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="border-b-row">
      <CollapsibleTrigger
        className="flex min-h-8 w-full items-center gap-2 rounded-ctl px-1 text-left fs-body hover:bg-row-hv"
        aria-label={`${name} · ${open ? t("hideChanges") : t("showChanges")}`}
      >
        <DisclosureChevron open={open} />
        <span className="truncate font-medium text-ink">{name}</span>
        {draft && <Tag>{t("draftTag")}</Tag>}
        {draft && path && <Mono className="truncate text-mut">{path}</Mono>}
        <span className="flex-1" />
        {diff?.state === "loading" ? (
          <RefreshIcon spinning />
        ) : (
          files.length > 0 && (
            <span className="shrink-0 fs-meta text-mut tabular-nums">
              +{added} −{removed}
            </span>
          )
        )}
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="pt-1 pb-3 pl-6">
          {diff?.state === "loading" && (
            <p className="flex items-center gap-2 fs-meta text-mut" role="status">
              <RefreshIcon spinning /> {t("diffLoading")}
            </p>
          )}
          {diff?.state === "error" && <p className="fs-meta text-mut">{t("diffFailed", { error: diff.error })}</p>}
          {diff?.state === "ok" &&
            (diff.value.files.length ? (
              <>
                {diff.value.base === false && !draft && <p className="mb-2 fs-meta leading-relaxed text-mut">{t("myChangesNoBase")}</p>}
                <SkillDiffView diff={diff.value} note={false} />
              </>
            ) : (
              <p className="fs-meta text-mut">{t("myChangesNone")}</p>
            ))}
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}
