import * as React from "react"
import { toast } from "sonner"
import { ListChecks, MoreHorizontal, Plus, RefreshCw, Trash2 } from "lucide-react"

import { cn, shortSha } from "@/lib/utils"
import { hasUpdates, isMine, useApp } from "@/lib/app-state"
import { backend, type Pack, type PackStatus, type Skill } from "@/lib/backend"
import { EmptyNote, Mono, Page, RefreshIcon, Sha, TableHead, Tag, UpdateDot } from "@/components/app/bits"
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
        <Button variant="outline" onClick={() => check()} disabled={checking}>
          <RefreshIcon spinning={checking} />
          {checking ? t("checking") : t("recheck")}
        </Button>
      }
    >
      {packs.length === 0 ? (
        <EmptyNote title={t("noSources")} action={<Button onClick={() => openDialog({ kind: "add" })}>{t("addSkill")}</Button>}>
          {t("noSourcesBody")}
        </EmptyNote>
      ) : (
        <div role="table" aria-label={t("sourcesTitle")} className="skd-cols-sources">
          <TableHead style={{ gridTemplateColumns: COLS }}>
            <div role="columnheader">{t("colSourceSkills")}</div>
            <div role="columnheader">{t("colVersion")}</div>
            <div role="columnheader">{t("colUsedOf")}</div>
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
          <LocalRow name={t("ownTitle")} hint={t("ownHint")} list={own} action={<Button variant="ghost" size="sm" onClick={() => openDialog({ kind: "new" })}><Plus />{t("newSkill")}</Button>} />
          {apps.map((app) => (
            <LocalRow key={app} name={app} hint={t("appsHint")} list={skills.filter((s) => s.kind === "app" && s.origin === app)} />
          ))}
        </div>
      )}
    </Page>
  )
}

function SkillLinks({ list }: { list: Skill[] }) {
  const { navigate, openDetail } = useApp()
  return (
    <span className="block font-mono fs-meta text-mut [overflow-wrap:anywhere]">
      {list.map((s, i) => (
        <React.Fragment key={s.ref.key}>
          {i > 0 && ", "}
          <button
            type="button"
            className="rounded-seg hover:text-ink hover:underline hover:underline-offset-[3px]"
            onClick={() => {
              navigate("skills")
              openDetail(s.ref.key)
            }}
          >
            {s.linkName}
          </button>
        </React.Fragment>
      ))}
    </span>
  )
}

function SourceRow({ pack: p, used, st, open, onToggle, failed }: { pack: Pack; used: Skill[]; st?: PackStatus; open: boolean; onToggle: () => void; failed?: string }) {
  const { t, openDialog, check } = useApp()
  const upd = !!st && hasUpdates(st)
  const movedOnly = !!st && !upd && st.moved.length > 0
  const total = Math.max(p.upstreamCount, used.length, 1)
  const reviewable = upd || movedOnly
  const statusText = reviewable ? (open ? t("collapse") : st!.moved.length ? t("hasUpdatesMoved") : t("hasUpdatesView")) : failed ? t("checkFailedShort") : st ? t("upToDate") : "—"

  return (
    <div id={`source-${p.repo}`} className={cn(open && "bg-row-hv")}>
      <div role="row" className="grid min-h-[61px] items-center border-b-row px-2.5 hover:bg-row-hv" style={{ gridTemplateColumns: COLS }}>
        <div role="cell" className="min-w-0 pr-4">
          <div className="flex items-center gap-2 font-heading fs-name leading-[1.2] font-semibold text-ink">
            <span className="truncate">{p.repo}</span>
            {p.followAll && <Tag>{t("followAll")}</Tag>}
            {p.alias && <Mono className="font-normal text-mut">{p.alias}</Mono>}
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
          <span className="font-mono fs-meta text-ink">
            {used.length} / {p.upstreamCount || "?"}
          </span>
        </div>
        <div role="cell" className="min-w-0 truncate fs-meta text-mut" title={p.license}>
          {p.license ?? "—"}
        </div>
        <div role="cell" className={cn("flex items-center fs-meta", reviewable ? "font-semibold text-ink" : "text-mut")}>
          {reviewable ? (
            <button type="button" aria-expanded={open} onClick={onToggle} className="flex items-center gap-1.5 whitespace-nowrap underline decoration-ln underline-offset-[3px]">
              {upd && <UpdateDot decorative />}
              {statusText}
            </button>
          ) : (
            <span title={failed}>{statusText}</span>
          )}
        </div>
        <div role="cell" className="flex justify-end gap-1">
          <Button variant="ghost" size="sm" title={t("chooseSkillsHint")} onClick={() => openDialog({ kind: "add", initial: p.repo })}>
            <ListChecks />
            {t("chooseSkills")}
          </Button>
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
              <DropdownMenuSeparator />
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
  const { t, runTask, reload, navigate, operation } = useApp()
  const [addNew, setAddNew] = React.useState<string[]>([])
  const usedNames = new Set(used.map((s) => s.name))
  const affected = [...st.changed, ...st.pending].filter((n) => usedNames.has(n))
  const impact: [string, string[]][] = [
    [t("skillChanged"), st.changed],
    [t("skillPending"), st.pending],
    [t("skillGone"), st.gone],
    [t("skillMoved"), st.moved.map((m) => m.split(": ")[0])],
  ]

  const go = async () => {
    const r = await runTask(t("updating"), () => backend.update([pack.repo], { addNew: addNew.length ? { [pack.repo]: addNew } : undefined }))
    if (!r.ok) return
    await reload()
    onClose()
    toast(t("updatedOne", { repo: pack.repo, to: shortSha(st.latest) }), { action: { label: t("navHistory"), onClick: () => navigate("history") } })
    const backups = r.value.flatMap((h) => h.backups)
    if (backups.length) toast(t("backupsToast", { n: backups.length }), { action: { label: t("openTrash"), onClick: () => navigate("trash") } })
  }

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_300px] gap-6 border-b-row bg-row-hv py-3.5 pr-2.5 pb-4 pl-[26px] [border-left:3px_solid_var(--ac)] max-[900px]:grid-cols-1">
      <div className="min-w-0">
        <div className="flex items-baseline gap-2.5 fs-meta text-mut">
          <span className="font-semibold text-ink">{t("upstreamCommits")}</span>
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
                  <Mono className="fs-body">{n}</Mono>
                  <span className="fs-meta text-mut">{t("addNewToo")}</span>
                </label>
              ))}
            </div>
          </fieldset>
        )}
      </div>
      <div>
        <div className="fs-meta text-mut">{t("affectsYours")}</div>
        <div className="mt-1 font-mono fs-body text-hue-ink">{affected.length ? affected.join(", ") : t("affectsNone")}</div>
        <dl className="mt-2 space-y-0.5 fs-meta">
          {impact
            .filter(([, l]) => l.length)
            .map(([label, l]) => (
              <div key={label} className="flex gap-2">
                <dt className="flex-none text-mut">{label}</dt>
                <dd className="min-w-0 truncate font-mono text-ink">{l.join(", ")}</dd>
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
      </div>
    </div>
  )
}

function LocalRow({ name, hint, list, action }: { name: string; hint: string; list: Skill[]; action?: React.ReactNode }) {
  return (
    <div role="row" className="grid min-h-[60px] items-center border-b-row px-2.5 hover:bg-row-hv" style={{ gridTemplateColumns: COLS }}>
      <div role="cell" className="min-w-0 pr-4">
        <div className="flex items-baseline gap-2 font-heading fs-name leading-[1.2] font-semibold text-ink">
          <span className="truncate">{name}</span>
          <span className="truncate font-body fs-meta font-normal text-mut">{hint}</span>
        </div>
        <div className="mt-0.5">
          <SkillLinks list={list} />
        </div>
      </div>
      <div role="cell" />
      <div role="cell" className="font-mono fs-meta text-ink">
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
