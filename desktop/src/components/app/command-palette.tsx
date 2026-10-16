import * as React from "react"
import {
  Bot,
  CloudUpload,
  Copy,
  FolderGit2,
  FolderOpen,
  History,
  Inbox,
  Languages,
  Layers,
  Link2,
  Monitor,
  Moon,
  PencilLine,
  Plus,
  RefreshCw,
  Settings,
  SquarePlus,
  Sun,
  Trash2,
  Download,
  Replace,
} from "lucide-react"

import { useApp, kindLabel, type View } from "@/lib/app-state"
import { useSkillActions } from "@/lib/use-actions"
import { UpdateDot } from "@/components/app/bits"
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator, CommandShortcut } from "@/components/ui/command"
import { Command } from "@/components/ui/command"

export function CommandPalette() {
  const { t, paletteOpen, setPaletteOpen, skills, navigate, select, selected, openDialog, check, updatablePacks, plan, trashCount, setConfig, config, openEditor, updatable } = useApp()
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

  return (
    <CommandDialog open={paletteOpen} onOpenChange={setPaletteOpen} title={t("commandPalette")} description={t("cmdPlaceholder")} className="sm:max-w-xl">
      <Command loop>
        <CommandInput placeholder={t("cmdPlaceholder")} value={search} onValueChange={setSearch} />
        <CommandList className="max-h-[22rem]">
          <CommandEmpty>{t("cmdEmpty")}</CommandEmpty>

          {current && (
            <CommandGroup heading={current.linkName}>
              <CommandItem keywords={["edit"]} onSelect={() => run(() => openEditor(current.ref))}>
                <PencilLine /> {t("edit")}
              </CommandItem>
              <CommandItem keywords={["reveal"]} onSelect={() => run(() => void reveal(current.ref))}>
                <FolderOpen /> {t("revealInFinder")}
              </CommandItem>
              {(current.kind === "vendor" || current.kind === "app") && (
                <CommandItem keywords={["fork"]} onSelect={() => run(() => openDialog({ kind: "fork", ref: current.ref }))}>
                  <Copy /> {t("forkSkill")}
                </CommandItem>
              )}
              {current.kind === "vendor" && (
                <CommandItem keywords={["trash"]} onSelect={() => run(() => openDialog({ kind: "remove", ref: current.ref }))}>
                  <Trash2 /> {t("moveToTrash")}
                </CommandItem>
              )}
            </CommandGroup>
          )}

          <CommandGroup heading={t("cmdGroupActions")}>
            <CommandItem keywords={["add skill"]} onSelect={() => run(() => openDialog({ kind: "add" }))}>
              <Plus /> {t("addSkill")}
            </CommandItem>
            <CommandItem keywords={["new skill"]} onSelect={() => run(() => openDialog({ kind: "new" }))}>
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
              <CommandItem keywords={["apply links"]} onSelect={() => run(() => openDialog({ kind: "apply" }))}>
                <Link2 /> {t("cmdApply")}
              </CommandItem>
            )}
            <CommandItem keywords={["sync backup github"]} onSelect={() => run(() => openDialog({ kind: "sync" }))}>
              <CloudUpload /> {t("cmdSync")}
            </CommandItem>
            <CommandItem keywords={["scan adopt"]} onSelect={() => go("scan")}>
              <Inbox /> {t("cmdScan")}
            </CommandItem>
            {trashCount > 0 && (
              <CommandItem keywords={["empty trash"]} onSelect={() => run(() => openDialog({ kind: "emptyTrash" }))}>
                <Trash2 /> {t("cmdEmptyTrash")}
              </CommandItem>
            )}
            <CommandItem keywords={["switch library use"]} onSelect={() => run(() => openDialog({ kind: "library", mode: "use" }))}>
              <Replace /> {t("cmdSwitchLibrary")}
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
                  })
                }
              >
                <Layers />
                <span className="truncate">{s.linkName}</span>
                {updatable.has(s.ref.key) && <UpdateDot />}
                <span className="ml-auto truncate pl-3 text-xs text-muted-foreground">{kindLabel(t, s)}</span>
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
            <CommandItem keywords={["theme light"]} onSelect={() => run(() => void setConfig({ theme: "light" }))}>
              <Sun /> {t("cmdThemeLight")}
            </CommandItem>
            <CommandItem keywords={["theme dark"]} onSelect={() => run(() => void setConfig({ theme: "dark" }))}>
              <Moon /> {t("cmdThemeDark")}
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
