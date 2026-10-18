import * as React from "react"
import {
  Bot,
  CloudUpload,
  Copy,
  Download,
  Filter,
  FolderGit2,
  FolderOpen,
  History,
  Inbox,
  Info,
  Languages,
  Layers,
  Link2,
  Monitor,
  Palette,
  PencilLine,
  Plus,
  RefreshCw,
  Replace,
  Settings,
  SquarePlus,
  Sun,
  Table2,
  Trash2,
  Undo2,
  Waypoints,
} from "lucide-react"

import { useApp, kindLabel, type SkillFilter, type View } from "@/lib/app-state"
import { useSkillActions } from "@/lib/use-actions"
import { UpdateDot } from "@/components/app/bits"
import { Command, CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator, CommandShortcut } from "@/components/ui/command"

/** ⌘K: every page, action and skill, plus the preferences. shadcn command (cmdk) in a dialog. */
export function CommandPalette() {
  const {
    t,
    paletteOpen,
    setPaletteOpen,
    skills,
    navigate,
    select,
    selected,
    openDetail,
    openDialog,
    check,
    updatablePacks,
    plan,
    setConfig,
    config,
    openEditor,
    updatable,
    style,
    setStyle,
    skillView,
    setSkillView,
    setFilter,
    setGrouped,
    grouped,
    pendingCount,
    applyPending,
    revertPending,
  } = useApp()
  const { reveal } = useSkillActions()
  const current = skills.find((s) => s.ref.key === selected)

  const [search, setSearch] = React.useState("")
  React.useEffect(() => {
    if (paletteOpen) setSearch("")
  }, [paletteOpen])

  const run = (fn: () => void) => {
    setPaletteOpen(false)
    fn()
  }
  const go = (v: View) => run(() => navigate(v))
  const showFilter = (f: SkillFilter) =>
    run(() => {
      navigate("skills")
      setFilter(f)
    })

  return (
    <CommandDialog open={paletteOpen} onOpenChange={setPaletteOpen} title={t("commandPalette")} description={t("cmdPlaceholder")}>
      <Command loop>
        <CommandInput placeholder={t("cmdPlaceholder")} value={search} onValueChange={setSearch} />
        <CommandList className="max-h-[24rem]">
          <CommandEmpty>{t("cmdEmpty")}</CommandEmpty>

          {pendingCount > 0 && (
            <CommandGroup heading={t("pendingN", { n: pendingCount })}>
              <CommandItem keywords={["apply pending"]} onSelect={() => run(() => void applyPending())}>
                <Link2 /> {t("apply")}
                <CommandShortcut>⏎</CommandShortcut>
              </CommandItem>
              <CommandItem keywords={["revert undo pending"]} onSelect={() => run(revertPending)}>
                <Undo2 /> {t("revert")}
              </CommandItem>
            </CommandGroup>
          )}

          {current && (
            <CommandGroup heading={current.linkName}>
              <CommandItem keywords={["details info"]} onSelect={() => run(() => openDetail(current.ref.key))}>
                <Info /> {t("details")}
                <CommandShortcut>␣</CommandShortcut>
              </CommandItem>
              <CommandItem keywords={["edit"]} onSelect={() => run(() => openEditor(current.ref))}>
                <PencilLine /> {t("edit")}
              </CommandItem>
              <CommandItem keywords={["reveal finder"]} onSelect={() => run(() => void reveal(current.ref))}>
                <FolderOpen /> {t("revealInFinder")}
              </CommandItem>
              {(current.kind === "vendor" || current.kind === "app") && (
                <CommandItem keywords={["fork copy"]} onSelect={() => run(() => openDialog({ kind: "fork", ref: current.ref }))}>
                  <Copy /> {t("forkSkill")}
                </CommandItem>
              )}
              {current.kind !== "app" ? (
                <CommandItem keywords={["trash remove"]} onSelect={() => run(() => openDialog({ kind: "remove", ref: current.ref }))}>
                  <Trash2 /> {t("moveToTrash")}
                </CommandItem>
              ) : (
                <CommandItem keywords={["unlink"]} onSelect={() => run(() => openDialog({ kind: "unlink", ref: current.ref }))}>
                  <Trash2 /> {t("unlinkAll")}
                </CommandItem>
              )}
            </CommandGroup>
          )}

          <CommandGroup heading={t("cmdGroupActions")}>
            <CommandItem keywords={["add skill install"]} onSelect={() => run(() => openDialog({ kind: "add" }))}>
              <Plus /> {t("addSkill")}
            </CommandItem>
            <CommandItem keywords={["new skill create"]} onSelect={() => run(() => openDialog({ kind: "new" }))}>
              <SquarePlus /> {t("newSkill")}
              <CommandShortcut>⌘N</CommandShortcut>
            </CommandItem>
            <CommandItem keywords={["check outdated"]} onSelect={() => run(() => void check())}>
              <RefreshCw /> {t("cmdCheck")}
            </CommandItem>
            {updatablePacks.length > 0 && (
              <CommandItem keywords={["update"]} onSelect={() => run(() => openDialog({ kind: "update" }))}>
                <Download /> {t("cmdUpdate")}
                <CommandShortcut>{t("updatesAvailable", { n: updatablePacks.length })}</CommandShortcut>
              </CommandItem>
            )}
            {plan.length > 0 && (
              <CommandItem keywords={["apply links drift"]} onSelect={() => run(() => openDialog({ kind: "apply" }))}>
                <Link2 /> {t("cmdApply")}
              </CommandItem>
            )}
            <CommandItem keywords={["sync backup github push"]} onSelect={() => run(() => openDialog({ kind: "sync" }))}>
              <CloudUpload /> {t("cmdSync")}
            </CommandItem>
            <CommandItem keywords={["scan adopt"]} onSelect={() => go("scan")}>
              <Inbox /> {t("cmdScan")}
            </CommandItem>
            <CommandItem keywords={["switch library use"]} onSelect={() => run(() => openDialog({ kind: "library", mode: "use" }))}>
              <Replace /> {t("cmdSwitchLibrary")}
            </CommandItem>
          </CommandGroup>

          <CommandSeparator />
          <CommandGroup heading={t("cmdGroupView")}>
            <CommandItem keywords={["wire view lines"]} onSelect={() => run(() => (navigate("skills"), setSkillView(skillView === "wire" ? "table" : "wire")))}>
              {skillView === "wire" ? <Table2 /> : <Waypoints />} {skillView === "wire" ? t("cmdViewTable") : t("cmdViewWire")}
              <CommandShortcut>V</CommandShortcut>
            </CommandItem>
            <CommandItem keywords={["group source"]} onSelect={() => run(() => (navigate("skills"), setGrouped(!grouped)))}>
              <Layers /> {grouped ? t("cmdUngroup") : t("groupBySource")}
            </CommandItem>
            <CommandItem keywords={["filter all"]} onSelect={() => showFilter("all")}>
              <Filter /> {t("cmdFilter", { name: t("filterAll") })}
            </CommandItem>
            <CommandItem keywords={["filter mine own"]} onSelect={() => showFilter("mine")}>
              <Filter /> {t("cmdFilter", { name: t("filterMine") })}
            </CommandItem>
            <CommandItem keywords={["filter vendor source"]} onSelect={() => showFilter("vendor")}>
              <Filter /> {t("cmdFilter", { name: t("filterVendor") })}
            </CommandItem>
            <CommandItem keywords={["filter updates"]} onSelect={() => showFilter("updates")}>
              <Filter /> {t("cmdFilter", { name: t("filterUpdates") })}
            </CommandItem>
          </CommandGroup>

          <CommandSeparator />
          <CommandGroup heading={t("cmdGroupSkills")}>
            {skills.map((s) => (
              <CommandItem
                key={s.ref.key}
                value={`${s.linkName} ${s.origin} ${s.kind}`}
                keywords={[s.description]}
                onSelect={() =>
                  run(() => {
                    navigate("skills")
                    select(s.ref.key)
                    openDetail(s.ref.key)
                  })
                }
              >
                <Layers />
                <span className="truncate">{s.linkName}</span>
                {updatable.has(s.ref.key) && <UpdateDot />}
                <span className="ml-auto truncate pl-3 fs-meta text-mut">{kindLabel(t, s)}</span>
              </CommandItem>
            ))}
          </CommandGroup>

          <CommandSeparator />
          <CommandGroup heading={t("cmdGroupGo")}>
            {(
              [
                ["skills", Layers, t("navSkills")],
                ["sources", FolderGit2, t("navSources")],
                ["agents", Bot, t("navAgents")],
                ["history", History, t("navHistory")],
                ["scan", Inbox, t("navScan")],
                ["trash", Trash2, t("navTrash")],
                ["settings", Settings, t("navSettings")],
              ] as const
            ).map(([v, Icon, label]) => (
              <CommandItem key={v} keywords={[v]} onSelect={() => go(v)}>
                <Icon /> {label}
                {v === "settings" && <CommandShortcut>⌘,</CommandShortcut>}
              </CommandItem>
            ))}
          </CommandGroup>

          <CommandSeparator />
          <CommandGroup heading={t("cmdGroupPrefs")}>
            <CommandItem keywords={["style theme doodle clean"]} onSelect={() => run(() => setStyle(style === "clean" ? "doodle" : "clean"))}>
              <Palette /> {style === "clean" ? t("cmdStyleDoodle") : t("cmdStyleClean")}
            </CommandItem>
            <CommandItem keywords={["theme light"]} onSelect={() => run(() => void setConfig({ theme: "light" }))}>
              <Sun /> {t("cmdThemeLight")}
            </CommandItem>
            <CommandItem keywords={["theme system"]} onSelect={() => run(() => void setConfig({ theme: "system" }))}>
              <Monitor /> {t("cmdThemeSystem")}
            </CommandItem>
            <CommandItem keywords={["language"]} onSelect={() => run(() => void setConfig({ lang: config.lang === "zh" ? "en" : "zh" }))}>
              <Languages /> {config.lang === "zh" ? t("cmdLangEn") : t("cmdLangZh")}
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </Command>
    </CommandDialog>
  )
}
