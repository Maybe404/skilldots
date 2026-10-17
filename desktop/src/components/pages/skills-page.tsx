import * as React from "react"
import {
  createColumnHelper,
  createSortedRowModel,
  rowSortingFeature,
  sortFn_alphanumeric,
  sortFn_text,
  tableFeatures,
  useTable,
  type SortingState,
} from "@tanstack/react-table"
import { ArrowDown, ArrowUp, Check, ChevronsUpDown, Columns3, SearchX } from "lucide-react"

import { cn } from "@/lib/utils"
import { PLACEHOLDER_DESC, useApp, useEnabledAgents, type SkillFilter } from "@/lib/app-state"
import { backend, type Agent, type Skill } from "@/lib/backend"
import { useSkillActions } from "@/lib/use-actions"
import { KindBadge, OffRing, UpdateDot } from "@/components/app/bits"
import { SkillDetail } from "@/components/app/skill-detail"
import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Toggle } from "@/components/ui/toggle"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

const features = tableFeatures({
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns: { alphanumeric: sortFn_alphanumeric, text: sortFn_text },
})
const col = createColumnHelper<typeof features, Skill>()

export function matchesFilter(s: Skill, filter: SkillFilter, updatable: Set<string>) {
  switch (filter) {
    case "mine":
      return s.kind === "skills" || s.kind === "private"
    case "vendor":
      return s.kind === "vendor" || s.kind === "app"
    case "updates":
      return updatable.has(s.ref.key)
    default:
      return true
  }
}

export function matchesQuery(s: Skill, q: string) {
  if (!q) return true
  const n = q.toLowerCase()
  return [s.linkName, s.name, s.origin, s.description, s.category].some((x) => x.toLowerCase().includes(n))
}

export function SkillsPage() {
  const { t, skills, agents: allAgents, updatable, overrides, query, setQuery, filter, setFilter, selected, select, run, reload } = useApp()
  const agents = useEnabledAgents(true)
  const { setAgent } = useSkillActions()
  const [sorting, setSorting] = React.useState<SortingState>([])

  const data = React.useMemo(
    () => skills.filter((s) => matchesFilter(s, filter, updatable) && matchesQuery(s, query.trim())),
    [skills, filter, updatable, query],
  )
  const counts = React.useMemo(
    () => ({
      all: skills.length,
      mine: skills.filter((s) => matchesFilter(s, "mine", updatable)).length,
      vendor: skills.filter((s) => matchesFilter(s, "vendor", updatable)).length,
      updates: skills.filter((s) => updatable.has(s.ref.key)).length,
    }),
    [skills, updatable],
  )

  const columns = React.useMemo(
    () =>
      col.columns([
        col.accessor("linkName", {
          id: "name",
          header: t("colName"),
          sortFn: "text",
          cell: ({ row }) => {
            const s = row.original
            return (
              <div className="min-w-0 py-0.5">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="inline-flex size-4 shrink-0 items-center justify-center">
                    {updatable.has(s.ref.key) && <UpdateDot />}
                  </span>
                  <span className="min-w-0 truncate font-medium">{s.linkName}</span>
                  {s.description === PLACEHOLDER_DESC && <span className="text-[0.75rem] text-muted-foreground">· {t("placeholderDesc")}</span>}
                </div>
                <p className="mt-0.5 truncate text-[0.8rem] text-muted-foreground">{s.description}</p>
              </div>
            )
          },
        }),
        col.accessor((s) => (s.kind === "vendor" ? s.origin : `~${s.kind}`), {
          id: "source",
          header: t("colSource"),
          sortFn: "text",
          cell: ({ row }) => <KindBadge skill={row.original} className="max-w-40 text-[0.8rem]" />,
        }),
        ...agents.map((a) =>
          col.display({
            id: `agent:${a.name}`,
            header: a.name === "agents" ? t("colAgents") : a.label,
            enableSorting: false,
            cell: ({ row }) => (
              <AgentCell skill={row.original} agent={a} off={a.name === "claude" && overrides.has(row.original.name)} onToggle={setAgent} />
            ),
          }),
        ),
      ]),
    // setAgent is recreated each render but stable in behaviour
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [agents, overrides, updatable, t],
  )

  const table = useTable({
    features,
    data,
    columns,
    getRowId: (s) => s.ref.key,
    onSortingChange: setSorting,
    state: { sorting },
  })

  const filtered = filter !== "all" || query.trim() !== ""
  const selectedSkill = skills.find((s) => s.ref.key === selected) ?? null
  const options: [SkillFilter, string, number][] = [
    ["all", t("filterAll"), counts.all],
    ["mine", t("filterMine"), counts.mine],
    ["vendor", t("filterVendor"), counts.vendor],
    ["updates", t("filterUpdates"), counts.updates],
  ]

  return (
    <div className="flex min-h-0 flex-1">
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center justify-between gap-4 px-8 pt-6 pb-4">
          <div>
            <h1 className="text-[1.35rem] leading-tight font-semibold tracking-tight">{t("navSkills")}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{t("skillsCount", { n: skills.length })}</p>
          </div>
          <div className="flex flex-wrap items-center justify-end gap-2">
            <ToggleGroup
              aria-label={t("filterLabel")}
              value={[filter]}
              onValueChange={(v) => v[0] && setFilter(v[0] as SkillFilter)}
              variant="outline"
              size="sm"
              spacing={0}
            >
              {options.map(([value, label, n]) => (
                <ToggleGroupItem key={value} value={value} className="gap-1.5 aria-pressed:bg-primary aria-pressed:text-primary-foreground data-[pressed]:bg-primary data-[pressed]:text-primary-foreground">
                  {label}
                  <span className="tabular-nums opacity-70">{n}</span>
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
            <DropdownMenu>
              <DropdownMenuTrigger render={<Button variant="outline" size="sm" aria-label={t("agentColumns")} />}>
                <Columns3 /> {t("agentColumns")}
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <div className="px-1.5 py-1 text-xs font-medium text-muted-foreground">{t("agentColumnsHint")}</div>
                {allAgents.filter((a) => a.enabled).map((a) => (
                  <DropdownMenuCheckboxItem
                    key={a.name}
                    checked={!a.hidden}
                    onCheckedChange={async (checked) => {
                      if ((await run(() => backend.setAgentHidden(a.name, !checked))).ok) await reload()
                    }}
                  >
                    {a.label}
                  </DropdownMenuCheckboxItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>

        <div className="flex min-h-0 flex-1 flex-col overflow-hidden px-8 pb-4">
          {data.length === 0 ? (
            <Empty className="mt-10 border border-dashed">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <SearchX />
                </EmptyMedia>
                <EmptyTitle>{t("noMatch")}</EmptyTitle>
                {query && <EmptyDescription>“{query}”</EmptyDescription>}
              </EmptyHeader>
              {filtered && (
                <EmptyContent>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      setFilter("all")
                      setQuery("")
                    }}
                  >
                    {t("clearFilters")}
                  </Button>
                </EmptyContent>
              )}
            </Empty>
          ) : (
            <div className="relative min-h-0 flex-1">
              <Table
                aria-label={t("navSkills")}
                className="table-fixed"
                containerClassName="skills-table-scroll min-h-0 h-full overflow-auto"
              >
                <colgroup>
                  <col className="w-[22rem]" />
                  <col className="w-[11rem]" />
                  {agents.map((a) => (
                    <col key={a.name} className="w-[5.4rem]" />
                  ))}
                </colgroup>
                <TableHeader className="bg-background">
                {table.getHeaderGroups().map((hg) => (
                  <TableRow key={hg.id} className="hover:bg-transparent">
                    {hg.headers.map((h, index) => {
                      const sorted = h.column.getIsSorted()
                      const isAgent = h.column.id.startsWith("agent:")
                      const pinned = index === 0 ? "sticky left-0 z-20 bg-background" : index === 1 ? "sticky left-[22rem] z-20 bg-background shadow-[1px_0_0_var(--border)]" : ""
                      return (
                        <TableHead
                          key={h.id}
                          aria-sort={sorted === "asc" ? "ascending" : sorted === "desc" ? "descending" : h.column.getCanSort() ? "none" : undefined}
                          className={cn("sticky top-0 z-30 h-11 border-b bg-background/95 text-[0.8rem] font-medium text-muted-foreground backdrop-blur", isAgent && "text-center", pinned)}
                        >
                          {h.column.getCanSort() ? (
                            <button
                              type="button"
                              className="-ml-1.5 inline-flex items-center gap-1 rounded-md px-1.5 py-1 hover:bg-muted hover:text-foreground"
                              onClick={h.column.getToggleSortingHandler()}
                            >
                              <table.FlexRender header={h} />
                              {sorted === "asc" ? <ArrowUp className="size-3" /> : sorted === "desc" ? <ArrowDown className="size-3" /> : <ChevronsUpDown className="size-3 opacity-50" />}
                            </button>
                          ) : (
                            <table.FlexRender header={h} />
                          )}
                        </TableHead>
                      )
                    })}
                  </TableRow>
                ))}
                </TableHeader>
                <TableBody>
                {table.getRowModel().rows.map((row) => (
                  <TableRow
                    key={row.id}
                    tabIndex={0}
                    aria-selected={selected === row.id}
                    data-state={selected === row.id ? "selected" : undefined}
                    className="cursor-default focus-visible:-outline-offset-2 data-[state=selected]:bg-muted"
                    onClick={() => select(row.id)}
                    onKeyDown={(e) => {
                      if (e.target !== e.currentTarget) return
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault()
                        select(row.id)
                      } else if (e.key === "ArrowDown") {
                        e.preventDefault()
                        ;(e.currentTarget.nextElementSibling as HTMLElement | null)?.focus()
                      } else if (e.key === "ArrowUp") {
                        e.preventDefault()
                        ;(e.currentTarget.previousElementSibling as HTMLElement | null)?.focus()
                      }
                    }}
                  >
                    {row.getAllCells().map((cell, index) => (
                      <TableCell
                        key={cell.id}
                        className={cn(
                          "py-2",
                          cell.column.id.startsWith("agent:") && "text-center",
                          index === 0 && "sticky left-0 z-[1] bg-background",
                          index === 1 && "sticky left-[22rem] z-[1] bg-background shadow-[1px_0_0_var(--border)]",
                        )}
                      >
                        <table.FlexRender cell={cell} />
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
                </TableBody>
              </Table>
              <div className="pointer-events-none absolute inset-x-0 bottom-0 flex justify-end px-3 pb-2">
                <span className="rounded-full border bg-background/90 px-2.5 py-1 text-[0.7rem] text-muted-foreground shadow-sm backdrop-blur sm:hidden">{t("tableScrollHint")}</span>
              </div>
            </div>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-x-5 gap-y-1 border-t px-8 py-2.5 text-[0.78rem] text-muted-foreground">
          <span className="inline-flex items-center gap-1.5">
            <span className="inline-flex size-4 items-center justify-center"><UpdateDot /></span> {t("legendUpdate")}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <OffRing /> {t("legendClaudeOff")}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="inline-flex size-3.5 items-center justify-center rounded-[4px] bg-foreground text-background">
              <Check className="size-2.5" strokeWidth={3} />
            </span>
            {t("legendCell")}
          </span>
        </div>
      </div>

      {selectedSkill && <SkillDetail skill={selectedSkill} onClose={() => select(null)} />}
    </div>
  )
}

/**
 * One skill × agent cell. Linked = a filled check square; not linked = a
 * faint dot that turns into an outline on hover/focus, so 19 × 5 cells read as
 * "where is it installed" instead of a wall of switches.
 */
function AgentCell({
  skill,
  agent,
  off,
  onToggle,
}: {
  skill: Skill
  agent: Agent
  off: boolean
  onToggle: (s: Skill, agent: string, on: boolean) => Promise<void>
}) {
  const { t } = useApp()
  const on = skill.targets.includes(agent.name)
  const [busy, setBusy] = React.useState(false)
  const label = on ? t("cellOn", { skill: skill.linkName, agent: agent.label }) : t("cellOff", { skill: skill.linkName, agent: agent.label })
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Toggle
            size="sm"
            aria-label={label + (off ? ` · ${t("claudeOffMarker")}` : "")}
            pressed={on}
            disabled={busy}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => e.stopPropagation()}
            onPressedChange={async (next) => {
              setBusy(true)
              await onToggle(skill, agent.name, next)
              setBusy(false)
            }}
            className="group/cell relative size-7 min-w-7 p-0 hover:bg-transparent aria-pressed:bg-transparent data-[pressed]:bg-transparent"
          />
        }
      >
        {on ? (
          <span className="inline-flex size-4 items-center justify-center rounded-[5px] bg-foreground text-background">
            <Check className="size-3" strokeWidth={3} />
          </span>
        ) : (
          <span className="inline-flex size-4 items-center justify-center rounded-[5px] border border-transparent group-hover/cell:border-foreground/45 group-focus-visible/cell:border-foreground/45">
            <span className="size-1.5 rounded-full border border-muted-foreground/45 bg-transparent group-hover/cell:border-foreground/70 group-focus-visible/cell:border-foreground/70" />
          </span>
        )}
        {off && <OffRing className="absolute -top-0.5 -right-0.5 size-2 bg-background" label={t("claudeOffMarker")} />}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
