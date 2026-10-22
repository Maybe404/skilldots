import * as React from "react"
import { toast } from "sonner"
import { Download, FileDiff, GitPullRequestArrow, ListChecks, MoreHorizontal, Plus, RefreshCw, Trash2, Upload, UserRoundCheck, UserRoundX } from "lucide-react"

import { cn, shortSha } from "@/lib/utils"
import { hasUpdates, isMine, isUnpushed, toastMergeOutcome, useApp } from "@/lib/app-state"
import { backend, type Pack, type PackStatus, type Skill } from "@/lib/backend"
import { EmptyNote, Hint, Mono, Page, RefreshIcon, Sha, TableHead, Tag, UpdateDot } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"

const COLS = "var(--cols)"

/** Sources (subscribed repositories): the design's table with an inline update review. */
export function SourcesPage() {
  const { t, packs, skills, outdated, check, checking, openDialog, sourceFocus, showSource } = useApp()
  const [expanded, setExpanded] = React.useState<Set<string>>(() => new Set(sourceFocus ? [sourceFocus] : []))

  // Opened from a skill's update notice: show that source's changes.
  React.useEffect(() => {
    if (!sourceFocus) return
    setExpanded((e) => new Set(e).add(sourceFocus))
    requestAnimationFrame(() => document.getElementById(`source-${sourceFocus}`)?.scrollIntoView({ block: "nearest" }))
    showSource(null)
  }, [sourceFocus, showSource])

  const status = (repo: string) => outdated?.packs.find((p) => p.repo === repo)
  const used = (p: Pack) => skills.filter((s) => s.kind === "vendor" && s.origin === p.repo)
  const upd = packs.filter((p) => {
    const st = status(p.repo)
    return st && hasUpdates(st)
  }).length
  const totalUsed = packs.reduce((n, p) => n + used(p).length, 0)
  const totalAll = packs.reduce((n, p) => n + Math.max(p.upstreamCount, used(p).length), 0)
  const own = skills.filter(isMine)
  const apps = [...new Set(skills.filter((s) => s.kind === "app").map((s) => s.origin))]

  const toggle = (repo: string) =>
    setExpanded((e) => {
      const n = new Set(e)
      if (n.has(repo)) n.delete(repo)
      else n.add(repo)
      return n
    })

  return (
    <Page
      title={t("sourcesTitle")}
      summary={t("sourcesSummary", { n: packs.length, upd, used: totalUsed, all: totalAll })}
      actions={
        <>
          <Button variant="outline" onClick={() => check()} disabled={checking}>
            <RefreshIcon spinning={checking} />
            {checking ? t("checking") : t("recheck")}
          </Button>
          {upd > 0 && (
            <Button onClick={() => openDialog({ kind: "update" })} disabled={checking}>
              <Download aria-hidden strokeWidth={1.8} />
              {t("updateAll")}
            </Button>
          )}
        </>
      }
    >
      {packs.length === 0 ? (
        <EmptyNote
          title={t("noSources")}
          action={
            <>
              <Button onClick={() => openDialog({ kind: "add" })}>{t("addSkill")}</Button>
              {/* Your own skills repository, added as yours ("这是我的仓库" ticked); it may still be empty. */}
              <Button variant="outline" onClick={() => openDialog({ kind: "add", own: true })}>
                {t("importOwnRepo")}
              </Button>
            </>
          }
        >
          {t("noSourcesBody")}
        </EmptyNote>
      ) : (
        <div role="table" aria-label={t("sourcesTitle")} className="skd-cols-sources">
          <TableHead style={{ gridTemplateColumns: COLS }}>
            <div role="columnheader">{t("colSourceSkills")}</div>
            <div role="columnheader">{t("colVersion")}</div>
            <div role="columnheader" className="truncate pr-2 whitespace-nowrap" title={t("colUsedOf")}>
              {t("colUsedOf")}
            </div>
            <div role="columnheader">{t("colLicense")}</div>
            <div role="columnheader">{t("colStatus")}</div>
            <div role="columnheader" />
          </TableHead>
          {packs.map((p) => (
            <SourceRow key={p.repo} pack={p} used={used(p)} st={status(p.repo)} open={expanded.has(p.repo)} onToggle={() => toggle(p.repo)} failed={outdated?.failed.find((f) => f.repo === p.repo)?.error} />
          ))}
        </div>
      )}

      {(own.length > 0 || apps.length > 0) && (
        <div role="table" aria-label={t("localTitle")} className="skd-cols-sources mt-8">
          <TableHead style={{ gridTemplateColumns: COLS }}>
            <div role="columnheader">{t("localTitle")}</div>
            <div role="columnheader" />
            <div role="columnheader">{t("colCount")}</div>
            <div role="columnheader" />
            <div role="columnheader" />
            <div role="columnheader" />
          </TableHead>
          <LocalRow name={t("ownTitle")} hint={t("ownHint")} list={own} action={<Hint label={t("newSkill")}><Button variant="ghost" size="sm" aria-label={t("newSkill")} className="max-[1099px]:w-[26px] max-[1099px]:px-0" onClick={() => openDialog({ kind: "new" })}><Plus /><span className="max-[1099px]:hidden">{t("newSkill")}</span></Button></Hint>} />
          {apps.map((app) => (
            <LocalRow key={app} name={app} hint={t("appsHint")} list={skills.filter((s) => s.kind === "app" && s.origin === app)} />
          ))}
        </div>
      )}
    </Page>
  )
}

function SkillLinks({ list }: { list: Skill[] }) {
  const { openDetail } = useApp()
  return (
    <span className="block fs-meta text-mut [overflow-wrap:anywhere]">
      {list.map((s, i) => (
        <React.Fragment key={s.ref.key}>
          {i > 0 && ", "}
          <Button
            variant="quiet"
            size="inline"
            className="inline rounded-seg whitespace-normal no-underline hover:underline"
            onClick={() => openDetail(s.ref.key)}
          >
            {s.linkName}
          </Button>
        </React.Fragment>
      ))}
    </span>
  )
}

function SourceRow({ pack: p, used, st, open, onToggle, failed }: { pack: Pack; used: Skill[]; st?: PackStatus; open: boolean; onToggle: () => void; failed?: string }) {
  const { t, openDialog, check, run, reload } = useApp()
  const upd = !!st && hasUpdates(st)
  const movedOnly = !!st && !upd && st.moved.length > 0
  const total = Math.max(p.upstreamCount, used.length, 1)
  const reviewable = upd || movedOnly
  const statusText = reviewable ? (open ? t("collapse") : st!.moved.length ? t("hasUpdatesMoved") : t(p.own ? "hasUpdatesOwn" : "hasUpdatesView")) : failed ? t("checkFailedShort") : st ? t("upToDate") : "—"
  // Skills changed or made here and not sent yet: your own repository pushes them from the row (推送 N 项).
  const unpushed = used.filter(isUnpushed).length
  const pushHere = !!p.own && unpushed > 0

  const toggleOwn = async () => {
    if (!(await run(() => backend.setOwn(p.repo, !p.own))).ok) return
    await reload().catch(() => undefined)
    toast(t(p.own ? "unmarkedOwnToast" : "markedOwnToast", { repo: p.repo }))
  }

  return (
    <div id={`source-${p.repo}`} className={cn(open && "bg-row-hv")}>
      <div role="row" className="grid min-h-[61px] items-center border-b-row px-2.5 hover:bg-row-hv" style={{ gridTemplateColumns: COLS }}>
        <div role="cell" className="min-w-0 pr-4">
          <div className="flex items-center gap-2 font-heading fs-name leading-[1.2] font-semibold text-ink">
            <span className="truncate">{p.repo}</span>
            {p.own && <Tag>{t("ownTag")}</Tag>}
            {p.followAll && <Tag>{t("followAll")}</Tag>}
            {p.alias && <span className="fs-meta font-normal text-mut">{p.alias}</span>}
          </div>
          <div className="mt-0.5">{used.length ? <SkillLinks list={used} /> : <span className="fs-meta text-mut">{t("noneUsed")}</span>}</div>
        </div>
        <div role="cell" className="min-w-0 truncate font-mono fs-meta text-mut" title={p.commit}>
          {shortSha(p.commit)}
        </div>
        <div role="cell" className="flex items-center gap-2.5">
          <span aria-hidden className="h-1 w-[70px] flex-none overflow-hidden rounded-[2px] bg-ln max-[1000px]:hidden">
            <span className="block h-full rounded-[2px] bg-hue" style={{ width: `${Math.max(4, Math.round((100 * used.length) / total))}%` }} />
          </span>
          <span className="fs-meta text-ink tabular-nums">
            {/* Drafts made in your own repository are not upstream yet: they count in both. */}
            {used.length} / {p.upstreamCount ? Math.max(p.upstreamCount, used.length) : "?"}
          </span>
        </div>
        <div role="cell" className="min-w-0 truncate fs-meta text-mut" title={p.license}>
          {p.license ?? "—"}
        </div>
        <div role="cell" className={cn("flex items-center fs-meta", reviewable ? "font-semibold text-ink" : "text-mut")}>
          {reviewable ? (
            <Button variant={null} size="inline" aria-expanded={open} onClick={onToggle} className="rounded-seg underline decoration-ln underline-offset-[3px] hover:decoration-current">
              {upd && <UpdateDot decorative />}
              {statusText}
            </Button>
          ) : (
            <span title={failed}>{statusText}</span>
          )}
        </div>
        <div role="cell" className="flex justify-end gap-1">
          {/* Narrow windows keep the icon: the name column needs the room more. With something to push, 调整选择 moves into the menu. */}
          {pushHere ? (
            <Hint label={t("pushNHint", { n: unpushed, repo: p.repo })}>
              <Button size="sm" aria-label={t("pushNHint", { n: unpushed, repo: p.repo })} className="max-[1099px]:w-[26px] max-[1099px]:px-0" onClick={() => openDialog({ kind: "contribute", repo: p.repo })}>
                <Upload />
                <span className="max-[1099px]:hidden">{t("pushN", { n: unpushed })}</span>
              </Button>
            </Hint>
          ) : (
            <Hint label={t("chooseSkillsHint")}>
              <Button variant="ghost" size="sm" aria-label={`${t("chooseSkills")} · ${p.repo}`} className="max-[1099px]:w-[26px] max-[1099px]:px-0" onClick={() => openDialog({ kind: "add", initial: p.repo, adjust: true })}>
                <ListChecks />
                <span className="max-[1099px]:hidden">{t("chooseSkills")}</span>
              </Button>
            </Hint>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label={t("moreFor", { name: p.repo })} />}>
              <MoreHorizontal className="size-[15px]" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-52">
              <DropdownMenuItem onClick={() => check([p.repo])}>
                <RefreshCw /> {t("checkThis")}
              </DropdownMenuItem>
              {reviewable && (
                <DropdownMenuItem onClick={() => openDialog({ kind: "update", repos: [p.repo] })}>
                  <ListChecks /> {t("updateDialogOpen")}
                </DropdownMenuItem>
              )}
              {pushHere && (
                <DropdownMenuItem onClick={() => openDialog({ kind: "add", initial: p.repo, adjust: true })}>
                  <ListChecks /> {t("chooseSkills")}
                </DropdownMenuItem>
              )}
              {/* Someone else's repository: every edit of its skills goes upstream at once from here. */}
              {!p.own && unpushed > 0 && (
                <DropdownMenuItem onClick={() => openDialog({ kind: "contribute", repo: p.repo })}>
                  <GitPullRequestArrow /> {t("contributeN", { n: unpushed })}
                </DropdownMenuItem>
              )}
              {p.own && (
                <DropdownMenuItem onClick={() => openDialog({ kind: "new", pack: p.repo })}>
                  <Plus /> {t("newInRepo")}
                </DropdownMenuItem>
              )}
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => void toggleOwn()}>
                {p.own ? <UserRoundX /> : <UserRoundCheck />} {t(p.own ? "unmarkOwn" : "markOwn")}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => openDialog({ kind: "purge", repo: p.repo })}>
                <Trash2 /> {t("removeSource")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
      {open && st && <UpdateReview pack={p} st={st} used={used} onClose={onToggle} />}
    </div>
  )
}

/** The expanded update review of one source (design: commits on the left, impact and actions on the right). */
function UpdateReview({ pack, st, used, onClose }: { pack: Pack; st: PackStatus; used: Skill[]; onClose: () => void }) {
  const { t, runTask, reload, navigate, operation, openDialog, skills, openResolve } = useApp()
  const [addNew, setAddNew] = React.useState<string[]>([])
  const usedNames = new Set(used.map((s) => s.name))
  const affected = [...st.changed, ...st.pending].filter((n) => usedNames.has(n))
  const impact: [string, string[]][] = [
    [t("skillChanged"), st.changed],
    [t("skillPending"), st.pending],
    [t("skillGone"), st.gone],
    [t("skillMoved"), st.moved.map((m) => m.split(": ")[0])],
    [t("skillLanded"), st.landed ?? []],
  ]

  const go = async () => {
    const r = await runTask(t("updating"), () => backend.update([pack.repo], { addNew: addNew.length ? { [pack.repo]: addNew } : undefined }))
    if (!r.ok) return
    await reload()
    onClose()
    toast(t("updatedOne", { repo: pack.repo, to: shortSha(st.latest) }), { action: { label: t("navHistory"), onClick: () => navigate("history") } })
    const backups = r.value.flatMap((h) => h.backups)
    if (backups.length) toast(t("backupsToast", { n: backups.length }), { action: { label: t("openTrash"), onClick: () => navigate("trash") } })
    toastMergeOutcome(t, r.value, skills, openResolve)
  }

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_300px] gap-6 border-b-row bg-row-hv py-3.5 pr-2.5 pb-4 pl-[26px] [border-left:3px_solid_var(--ac)] max-[900px]:grid-cols-1">
      <div className="min-w-0">
        <div className="flex items-baseline gap-2.5 fs-meta text-mut">
          <span className="font-semibold text-ink">{t(pack.own ? "repoCommits" : "upstreamCommits")}</span>
          <Mono title={`${st.commit} → ${st.latest}`}>
            {shortSha(st.commit)} → {shortSha(st.latest)}
          </Mono>
        </div>
        <div className="mt-2 flex flex-col gap-[5px]">
          {st.commits.map((c) => (
            <div key={c.id} className="flex gap-3 fs-body">
              <Sha id={c.id} className="w-16 flex-none text-mut" />
              <span className="min-w-0 text-ink">{c.title}</span>
            </div>
          ))}
          {st.commitsUnknown && <p className="fs-meta text-mut">{t("historyCommitsUnknown")}</p>}
          {st.moreCommits && <p className="fs-meta text-mut">{t("historyMoreCommits")}</p>}
        </div>
        {st.moved.length > 0 && <p className="mt-2.5 fs-meta text-ink">{t("movedNote")}</p>}
        {st.new.length > 0 && (
          <fieldset className="mt-3">
            <legend className="fs-meta text-mut">{t("upstreamNew")}</legend>
            <div className="mt-1.5 flex flex-wrap gap-x-5 gap-y-1.5">
              {st.new.map((n) => (
                <label key={n} className="flex items-center gap-2 fs-body text-ink">
                  <Checkbox checked={addNew.includes(n)} onCheckedChange={(v) => setAddNew((l) => (v ? [...l, n] : l.filter((x) => x !== n)))} />
                  <span className="font-medium">{n}</span>
                  <span className="fs-meta text-mut">{t("addNewToo")}</span>
                </label>
              ))}
            </div>
          </fieldset>
        )}
      </div>
      <div>
        <div className="fs-meta text-mut">{t("affectsYours")}</div>
        <div className="mt-1 fs-body font-medium text-hue-ink">{affected.length ? affected.join(", ") : t("affectsNone")}</div>
        <dl className="mt-2 space-y-0.5 fs-meta">
          {impact
            .filter(([, l]) => l.length)
            .map(([label, l]) => (
              <div key={label} className="flex gap-2">
                <dt className="flex-none text-mut">{label}</dt>
                <dd className="min-w-0 truncate text-ink">{l.join(", ")}</dd>
              </div>
            ))}
        </dl>
        <div className="mt-3.5 flex flex-wrap gap-2">
          <Button onClick={go} disabled={!!operation} className="px-3">
            {t("updateTo")} <span className="font-mono font-medium" title={st.latest}>{shortSha(st.latest)}</span>
          </Button>
          <Button variant="ghost" onClick={onClose}>
            {t("notNow")}
          </Button>
        </div>
        {/* File by file, with the skills you edited marked, in the update dialog. */}
        <Button variant="link" size="inline" className="mt-3 fs-meta" onClick={() => openDialog({ kind: "update", repos: [pack.repo] })}>
          <FileDiff aria-hidden />
          {t("seeFileChanges")}
        </Button>
      </div>
    </div>
  )
}

function LocalRow({ name, hint, list, action }: { name: string; hint: string; list: Skill[]; action?: React.ReactNode }) {
  return (
    <div role="row" className="grid min-h-[60px] items-center border-b-row px-2.5 hover:bg-row-hv" style={{ gridTemplateColumns: COLS }}>
      <div role="cell" className="min-w-0 pr-4">
        {/* The name never truncates; the hint gives way, and goes below 1000px. */}
        <div className="flex min-w-0 items-baseline gap-2 font-heading fs-name leading-[1.2] font-semibold text-ink">
          <span className="shrink-0 whitespace-nowrap">{name}</span>
          <span className="min-w-0 truncate font-body fs-meta font-normal text-mut max-[999px]:hidden" title={hint}>
            {hint}
          </span>
        </div>
        <div className="mt-0.5">
          <SkillLinks list={list} />
        </div>
      </div>
      <div role="cell" />
      <div role="cell" className="fs-meta text-ink tabular-nums">
        {list.length}
      </div>
      <div role="cell" />
      <div role="cell" />
      <div role="cell" className="flex justify-end">
        {action}
      </div>
    </div>
  )
}
