import { FolderGit2, ListChecks, MoreHorizontal, RefreshCw, Trash2 } from "lucide-react"

import { hasUpdates, useApp } from "@/lib/app-state"
import { Mono, PageHeader, Section, UpdateDot } from "@/components/app/bits"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Empty, EmptyContent, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"

export function SourcesPage() {
  const { t, packs, skills, outdated, openDialog, check, checking, navigate, select } = useApp()
  const own = skills.filter((s) => s.kind === "skills" || s.kind === "private")
  const apps = skills.filter((s) => s.kind === "app")

  const show = (key: string) => {
    navigate("skills")
    select(key)
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <PageHeader
        title={t("sourcesTitle")}
        hint={t("sourcesHint")}
        actions={
          <Button variant="outline" size="sm" onClick={() => check()} disabled={checking}>
            <RefreshCw className={checking ? "animate-spin motion-reduce:animate-none" : undefined} /> {t("recheck")}
          </Button>
        }
      />

      <div className="px-8 pb-8">
        {packs.length === 0 ? (
          <Empty className="border border-dashed">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <FolderGit2 />
              </EmptyMedia>
              <EmptyTitle>{t("noSources")}</EmptyTitle>
            </EmptyHeader>
            <EmptyContent>
              <Button size="sm" onClick={() => openDialog({ kind: "add" })}>
                {t("addSkill")}
              </Button>
            </EmptyContent>
          </Empty>
        ) : (
          <ul className="divide-y rounded-xl border">
            {packs.map((p) => {
              const used = skills.filter((s) => s.kind === "vendor" && s.origin === p.repo)
              const st = outdated?.packs.find((x) => x.repo === p.repo)
              const upd = st ? hasUpdates(st) : false
              const movedOnly = st && !upd && st.moved.length > 0
              return (
                <li key={p.repo} className="grid gap-3 px-4 py-3.5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start sm:gap-4">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
                      <span className="font-medium">{p.repo}</span>
                      {p.alias && <Mono className="text-muted-foreground">{p.alias}</Mono>}
                      {p.followAll && (
                        <Badge variant="outline" className="font-normal">
                          {t("followAll")}
                        </Badge>
                      )}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[0.8rem] text-muted-foreground">
                      <Mono>{p.commit}</Mono>
                      <span>{t("usedOf", { used: used.length, all: p.upstreamCount })}</span>
                      {p.license && <span>{p.license}</span>}
                      <span className="inline-flex items-center gap-1.5 text-foreground">
                        {upd ? (
                          <>
                            <UpdateDot /> {t("hasUpdates")}
                          </>
                        ) : movedOnly ? (
                          <span className="text-muted-foreground">{t("pathOnly")}</span>
                        ) : st ? (
                          <span className="text-muted-foreground">{t("upToDate")}</span>
                        ) : null}
                      </span>
                    </div>
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {used.map((s) => (
                        <Button key={s.ref.key} variant="secondary" size="xs" className="font-normal" onClick={() => show(s.ref.key)}>
                          {s.linkName}
                        </Button>
                      ))}
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center justify-end gap-1.5 sm:max-w-[23rem]">
                    {(upd || movedOnly) && (
                      <Button size="sm" onClick={() => openDialog({ kind: "update", repos: [p.repo] })}>
                        {t("update")}
                      </Button>
                    )}
                    <Button variant="outline" size="sm" onClick={() => openDialog({ kind: "add", initial: p.repo })}>
                      <ListChecks /> {t("chooseSkills")}
                    </Button>
                    <DropdownMenu>
                      <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label={t("more")} />}>
                        <MoreHorizontal />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-44">
                        <DropdownMenuItem onClick={() => check([p.repo])}>
                          <RefreshCw /> {t("check")}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onClick={() => openDialog({ kind: "purge", repo: p.repo })}>
                          <Trash2 /> {t("removeSource")}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      <Section title={t("ownTitle")} hint={t("ownHint")}>
        <div className="flex flex-wrap gap-1.5">
          {own.map((s) => (
            <Button key={s.ref.key} variant="outline" size="sm" className="font-normal" onClick={() => show(s.ref.key)}>
              {s.linkName}
              <span className="text-muted-foreground">{s.kind === "private" ? t("kindPrivate") : "skills/"}</span>
            </Button>
          ))}
          <Button variant="ghost" size="sm" onClick={() => openDialog({ kind: "new" })}>
            + {t("newSkill")}
          </Button>
        </div>
      </Section>

      {apps.length > 0 && (
        <Section title={t("appsTitle")} hint={t("appsHint")}>
          <ul className="divide-y rounded-xl border">
            {apps.map((s) => (
              <li key={s.ref.key} className="flex items-center justify-between gap-4 px-4 py-3">
                <div className="min-w-0">
                  <button type="button" className="font-medium hover:underline" onClick={() => show(s.ref.key)}>
                    {s.linkName}
                  </button>
                  <Mono className="block truncate text-muted-foreground">{s.dir}</Mono>
                </div>
              </li>
            ))}
          </ul>
        </Section>
      )}
    </div>
  )
}
