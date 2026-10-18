import { isMyGo } from "mygo-runtime"

import { useApp, type Style, type View } from "@/lib/app-state"
import type { MessageKey } from "@/lib/i18n"
import { cn } from "@/lib/utils"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
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

/** The style switch (简洁 / 涂鸦), in the top bar and in Settings. */
export function StyleSwitch({ className, itemClassName }: { className?: string; itemClassName?: string }) {
  const { t, style, setStyle } = useApp()
  return (
    <ToggleGroup aria-label={t("styleLabel")} value={[style]} onValueChange={(v) => v[0] && setStyle(v[0] as Style)} className={cn("skd-seg items-start", className)} spacing={2}>
      <ToggleGroupItem value="clean" variant="seg" size="none" className={itemClassName}>
        {t("styleClean")}
      </ToggleGroupItem>
      <ToggleGroupItem value="doodle" variant="seg" size="none" className={itemClassName}>
        {t("styleDoodle")}
      </ToggleGroupItem>
    </ToggleGroup>
  )
}

/**
 * Top bar from the design: brand and library, page navigation as a segmented
 * control, the style switch, skill search (⌘F) with the palette shortcut, and
 * 添加. Page links are buttons with aria-current; they switch views, not tabs.
 */
export function Header() {
  const { t, view, navigate, query, setQuery, status, openDialog, updatablePacks, setPaletteOpen } = useApp()
  const libName = status?.root?.split("/").filter(Boolean).pop() ?? ""
  return (
    <header className="relative z-[5] -mb-px flex h-[55px] shrink-0 items-center gap-2.5 border-b-hd px-6 max-[760px]:px-3" data-drag-region>
      <span className="font-heading fs-brand leading-none font-semibold text-ink">skilldots</span>
      <span className="ml-1 truncate font-mono fs-meta text-mut max-[1180px]:hidden" title={status?.root ?? ""}>
        {libName}
      </span>
      {/* Browser preview only; the native window has no such tag. */}
      {!isMyGo() && (
        <span title={t("exampleMode")} className="shrink-0 max-[1180px]:hidden">
          <Tag>{t("sampleNotice")}</Tag>
        </span>
      )}
      <span className="flex-1" />
      <nav aria-label={t("mainNav")} className="skd-seg h-9 min-w-0 font-body fs-body">
        {NAV.map(([v, key]) => (
          <button
            key={v}
            type="button"
            aria-current={view === v || (v === "skills" && view === "editor") ? "page" : undefined}
            onClick={() => navigate(v)}
            className="skd-seg-item h-6 px-[11px] max-[900px]:px-2"
          >
            {t(key)}
            {v === "sources" && updatablePacks.length > 0 && <span role="img" aria-label={t("updatesAvailable", { n: updatablePacks.length })} className="size-1.5 rounded-full bg-ac" />}
          </button>
        ))}
      </nav>
      <span className="flex-1" />
      <StyleSwitch className="h-[30px] p-0.5 max-[1060px]:hidden" />
      <label className="flex h-[34px] w-[246px] items-center gap-2 rounded-btn border-in bg-in px-3 font-body fs-body text-mut max-[1240px]:w-[190px] max-[980px]:w-[150px]">
        <svg aria-hidden width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" className="shrink-0">
          <circle cx="11" cy="11" r="7" />
          <path d="m20 20-3.5-3.5" />
        </svg>
        <input
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
          className="min-w-0 flex-1 border-none bg-transparent text-ink outline-none placeholder:text-mut"
          spellCheck={false}
          autoComplete="off"
        />
        <button
          type="button"
          onClick={() => setPaletteOpen(true)}
          aria-label={`${t("commandPalette")} ⌘K`}
          title={t("commandPalette")}
          className="shrink-0 rounded-seg px-0.5 font-mono text-[10.5px] text-mut hover:text-ink"
        >
          ⌘K
        </button>
      </label>
      <button
        type="button"
        onClick={() => openDialog({ kind: "add" })}
        className="flex h-8 shrink-0 items-center gap-1.5 rounded-btn bg-pri pr-3.5 pl-[11px] font-body fs-body font-semibold whitespace-nowrap text-pri-fg hover:opacity-90"
      >
        <svg aria-hidden width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round">
          <path d="M12 5v14M5 12h14" />
        </svg>
        {t("add")}
      </button>
    </header>
  )
}
