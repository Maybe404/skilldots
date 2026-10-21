import { isMyGo } from "mygo-runtime"
import { Plus, Search } from "lucide-react"

import { useApp, type Style, type View } from "@/lib/app-state"
import { tildify } from "@/lib/agents"
import type { MessageKey } from "@/lib/i18n"
import { cn } from "@/lib/utils"
import { Highlight, HighlightItem } from "@/components/animate-ui/primitives/effects/highlight"
import { Button } from "@/components/ui/button"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Kbd } from "@/components/ui/kbd"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { toggleThumb } from "@/components/ui/toggle"
import { Tag } from "@/components/app/bits"

export const searchInputId = "skd-search"

const NAV: [View, MessageKey][] = [
  ["skills", "navSkills"],
  ["sources", "navSources"],
  ["agents", "navAgents"],
  ["history", "navHistory"],
  ["scan", "navScan"],
  ["trash", "navTrash"],
  ["settings", "navSettings"],
]

/** The style switch (简洁 / 涂鸦), in Settings. */
export function StyleSwitch({ className, itemClassName }: { className?: string; itemClassName?: string }) {
  const { t, style, setStyle } = useApp()
  return (
    <ToggleGroup aria-label={t("styleLabel")} variant="seg" size="none" value={[style]} onValueChange={(v) => v[0] && setStyle(v[0] as Style)} className={cn("skd-seg", className)} spacing={2}>
      <ToggleGroupItem value="clean" className={itemClassName}>
        {t("styleClean")}
      </ToggleGroupItem>
      <ToggleGroupItem value="doodle" className={itemClassName}>
        {t("styleDoodle")}
      </ToggleGroupItem>
    </ToggleGroup>
  )
}

/**
 * Top bar from the design: brand and library, page navigation as a segmented
 * control, the style switch, skill search (⌘F) with the palette shortcut, and
 * 添加. Page links are buttons with aria-current; they switch views, not tabs,
 * and Animate UI's highlight slides the raised thumb to the current page.
 *
 * In the native window this is also the title bar (.skd-titlebar): it drags
 * the window and leaves room for the traffic lights on its left.
 *
 * The style switch (a rarely changed preference) lives in Settings only.
 *
 * Narrow windows (the native left padding is 94px, English labels are wide):
 * the nav never shrinks, it only trims its item padding; the search box
 * gives way first (down to 132px), then the library name truncates (it
 * hides only below 760px, where the window is a sliver), and 添加 / Add
 * becomes the + icon at 900px or less.
 */
export function Header() {
  const { t, view, navigate, query, setQuery, status, openDialog, updatablePacks, setPaletteOpen } = useApp()
  const libName = status?.root?.split("/").filter(Boolean).pop() ?? ""
  const current = view === "editor" || view === "resolve" ? "skills" : view
  return (
    <header className="skd-titlebar relative z-[5] -mb-px flex h-[55px] shrink-0 items-center gap-2.5 border-b-hd max-[900px]:gap-2 pr-6 max-[760px]:pr-3 max-[760px]:[--bar-pad:12px]">
      <span className="shrink-0 font-heading fs-brand leading-none font-semibold text-ink">skilldots</span>
      <span className="ml-1 min-w-[3.5rem] max-w-[180px] shrink truncate fs-meta text-mut max-[760px]:hidden" title={tildify(status?.root ?? "")}>
        {libName}
      </span>
      {/* Browser preview only; the native window has no such tag. */}
      {!isMyGo() && (
        <span title={t("exampleMode")} className="shrink-0 max-[1180px]:hidden">
          <Tag>{t("sampleNotice")}</Tag>
        </span>
      )}
      <span className="flex-1" />
      <nav aria-label={t("mainNav")} className="skd-seg h-8 shrink-0 font-body fs-body">
        <Highlight controlledItems value={current} click={false} exitDelay={0} className={toggleThumb.seg} style={{ inset: 0 }} transition={{ type: "spring", stiffness: 420, damping: 36 }}>
          {NAV.map(([v, key]) => (
            <HighlightItem key={v} value={v} className="flex rounded-seg">
              <button
                type="button"
                aria-current={current === v ? "page" : undefined}
                onClick={() => navigate(v)}
                className="skd-seg-item h-[26px] px-[11px] max-[1240px]:px-[9px] max-[1000px]:px-2 max-[900px]:px-[7px]"
              >
                {t(key)}
                {v === "sources" && updatablePacks.length > 0 && <span role="img" aria-label={t("updatesAvailable", { n: updatablePacks.length })} className="size-1.5 rounded-full bg-ac" />}
              </button>
            </HighlightItem>
          ))}
        </Highlight>
      </nav>
      <span className="flex-1" />
      <InputGroup variant="search" className="w-[246px] min-w-[132px] shrink font-body fs-body">
        <InputGroupAddon className="pl-3">
          <Search aria-hidden className="size-3.5" strokeWidth={1.8} />
        </InputGroupAddon>
        <InputGroupInput
          id={searchInputId}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
            if (view !== "skills") navigate("skills")
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              // Handled here: the Skill page's window-level Esc (which reverts pending changes) must not also run.
              e.preventDefault()
              setQuery("")
              ;(e.target as HTMLInputElement).blur()
            } else if (e.key === "ArrowDown" || e.key === "Enter") {
              e.preventDefault()
              document.querySelector<HTMLElement>("[id^=skill-row-][tabindex='0']")?.focus()
            }
          }}
          placeholder={t("searchPlaceholder")}
          aria-label={t("search")}
          className="h-full px-0 focus-visible:outline-none"
          spellCheck={false}
          autoComplete="off"
        />
        <InputGroupAddon align="inline-end" className="pr-px has-[>button]:mr-0">
          <InputGroupButton onClick={() => setPaletteOpen(true)} aria-label={`${t("commandPalette")} ⌘K`} title={t("commandPalette")} className="rounded-seg px-1 hover:bg-transparent">
            <Kbd>⌘K</Kbd>
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
      <Button size="pill" className="shrink-0 max-[901px]:px-[9px]" aria-label={t("add")} onClick={() => openDialog({ kind: "add" })}>
        <Plus aria-hidden className="size-3.5" strokeWidth={2.4} />
        <span className="max-[901px]:sr-only">{t("add")}</span>
      </Button>
    </header>
  )
}
