import * as React from "react"
import {
  Bot,
  CloudUpload,
  Command as CommandIcon,
  Loader2,
  FolderGit2,
  History,
  Inbox,
  Layers,
  Plus,
  RefreshCw,
  Search,
  Settings,
  Trash2,
} from "lucide-react"

import { cn } from "@/lib/utils"
import { useApp, type View } from "@/lib/app-state"
import { ago } from "@/lib/i18n"
import { Mono, UpdateDot } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Kbd } from "@/components/ui/kbd"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

export function AppSidebar() {
  const { t, view, navigate, skills, packs, agents, trashCount, updatablePacks, status, openDialog } = useApp()
  const libName = status?.root?.split("/").pop() ?? ""

  const item = (v: View, icon: React.ReactNode, label: string, badge?: React.ReactNode) => (
    <SidebarMenuItem key={v}>
      <SidebarMenuButton isActive={view === v} tooltip={label} onClick={() => navigate(v)} aria-current={view === v ? "page" : undefined}>
        {icon}
        <span>{label}</span>
      </SidebarMenuButton>
      {badge !== undefined && <SidebarMenuBadge className="text-muted-foreground">{badge}</SidebarMenuBadge>}
    </SidebarMenuItem>
  )

  return (
    <Sidebar variant="inset" collapsible="icon" aria-label={t("mainNav")}>
      {/* Space for the window's traffic lights; the native shell makes it draggable. */}
      <SidebarHeader className="pt-9" data-drag-region>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" tooltip={status?.root ?? ""} onClick={() => navigate("settings")} className="gap-2.5">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-foreground text-background">
                <Layers className="size-4" />
              </span>
              <span className="grid min-w-0 leading-tight">
                <span className="truncate font-semibold">{t("appName")}</span>
                <Mono className="truncate text-[0.72rem] text-muted-foreground">{libName}</Mono>
              </span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>{t("navLibrary")}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {item("skills", <Layers />, t("navSkills"), skills.length)}
              {item(
                "sources",
                <FolderGit2 />,
                t("navSources"),
                updatablePacks.length ? (
                  <span className="flex items-center gap-1.5">
                    <UpdateDot label={t("updatesAvailable", { n: updatablePacks.length })} />
                    {packs.length}
                  </span>
                ) : (
                  packs.length
                ),
              )}
              {item("agents", <Bot />, t("navAgents"), agents.filter((a) => a.enabled).length)}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>{t("navMaintain")}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {item("history", <History />, t("navHistory"))}
              {item("scan", <Inbox />, t("navScan"))}
              {item("trash", <Trash2 />, t("navTrash"), trashCount || undefined)}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton tooltip={t("backUp")} onClick={() => openDialog({ kind: "sync" })} className="h-auto py-2">
              <CloudUpload />
              <span className="grid min-w-0 leading-tight">
                <span className="truncate">{t("backUp")}</span>
                <span className="truncate text-[0.72rem] text-muted-foreground">
                  {status?.changes ? t("unsynced", { n: status.changes }) : t("synced")}
                </span>
              </span>
            </SidebarMenuButton>
          </SidebarMenuItem>
          {item("settings", <Settings />, t("navSettings"))}
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}

const TITLES: Record<View, Parameters<ReturnType<typeof useApp>["t"]>[0]> = {
  skills: "navSkills",
  sources: "navSources",
  agents: "navAgents",
  history: "navHistory",
  scan: "navScan",
  trash: "navTrash",
  settings: "navSettings",
  editor: "edit",
}

export const searchInputId = "toolbar-search"

/** Glass toolbar over the content panel. */
export function Toolbar() {
  const { t, view, navigate, query, setQuery, outdated, checking, check, updatablePacks, openDialog, setPaletteOpen } = useApp()
  return (
    <header className="glass-bar sticky top-0 z-20 flex h-13 shrink-0 items-center gap-2 rounded-t-xl px-3" data-drag-region>
      <SidebarTrigger aria-label={t("toggleSidebar")} />
      <span className="sr-only">{t(TITLES[view])}</span>
      <div className="ml-auto flex items-center gap-2">
        <InputGroup className="h-8 w-60 bg-background/60">
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
          <InputGroupInput
            id={searchInputId}
            aria-label={t("search")}
            placeholder={t("searchPlaceholder")}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              if (view !== "skills") navigate("skills")
            }}
            onKeyDown={(e) => {
              if (e.key === "Escape") setQuery("")
            }}
          />
        </InputGroup>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button variant="ghost" size="sm" onClick={() => check()} disabled={checking} className="text-muted-foreground" aria-label={`${t("recheck")} · ${t("checkOnlyHint")}`}>
                <RefreshCw className={cn(checking && "animate-spin motion-reduce:animate-none")} />
                <span className="text-[0.78rem]">
                  {checking ? t("checking") : outdated ? t("checkedAgo", { ago: ago(outdated.checked, t) }) : t("recheck")}
                </span>
              </Button>
            }
          />
          <TooltipContent>
            {t("recheck")} · {t("checkOnlyHint")}
          </TooltipContent>
        </Tooltip>

        {updatablePacks.length > 0 && (
          <Button variant="outline" size="sm" onClick={() => openDialog({ kind: "update" })}>
            <UpdateDot decorative />
            {t("updatesAvailable", { n: updatablePacks.length })}
          </Button>
        )}

        <Button size="sm" onClick={() => openDialog({ kind: "add" })}>
          <Plus /> {t("add")}
        </Button>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button variant="ghost" size="icon-sm" onClick={() => setPaletteOpen(true)} aria-label={t("commandPalette")}>
                <CommandIcon />
              </Button>
            }
          />
          <TooltipContent>
            {t("commandPalette")} <Kbd>⌘K</Kbd>
          </TooltipContent>
        </Tooltip>
      </div>
    </header>
  )
}

/** Shown under the toolbar when the manifest and the links differ. */
export function PendingBanner() {
  const { t, plan, openDialog } = useApp()
  if (!plan.length) return null
  return (
    <div className="flex items-center gap-3 border-b bg-muted/60 px-8 py-2 text-[0.82rem]">
      <span className="inline-block size-1.5 rounded-full bg-foreground" aria-hidden />
      <span>{t("pendingLinks", { n: plan.length })}</span>
      <Button variant="link" size="sm" className="h-auto p-0 text-[0.82rem] underline" onClick={() => openDialog({ kind: "apply" })}>
        {t("reviewApply")}
      </Button>
    </div>
  )
}

/** Keeps long native operations visible instead of making the window look stuck. */
export function OperationBanner() {
  const { t, operation } = useApp()
  const [elapsed, setElapsed] = React.useState(0)
  React.useEffect(() => {
    if (!operation) {
      setElapsed(0)
      return
    }
    const tick = () => setElapsed(Math.max(0, Math.floor((Date.now() - operation.startedAt) / 1000)))
    tick()
    const timer = window.setInterval(tick, 1000)
    return () => window.clearInterval(timer)
  }, [operation])
  if (!operation) return null
  return (
    <div className="flex items-center gap-2 border-b bg-muted/50 px-8 py-2 text-[0.8rem]" role="status" aria-live="polite">
      <Loader2 className="size-3.5 animate-spin motion-reduce:animate-none" aria-hidden />
      <span>{operation.label}</span>
      <span className="text-muted-foreground">{elapsed}s · {t("operationWait")}</span>
    </div>
  )
}
