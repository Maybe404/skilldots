import * as React from "react"

import { cn } from "@/lib/utils"
import { useApp } from "@/lib/app-state"
import { StyleSwitch } from "@/components/app/header"
import { LibraryForm } from "@/components/dialogs/ops-dialogs"

type Mode = "init" | "clone" | "use"

/**
 * First run (not drawn in the design): the top bar's brand and style switch,
 * a page title like the other pages, and the three ways to get a library as
 * radio rows in the design's table style.
 */
export function Onboarding() {
  const { t, status, navigate } = useApp()
  const [mode, setMode] = React.useState<Mode>("init")
  const options: [Mode, string, string][] = [
    ["init", t("obNew"), t("obNewHint")],
    ["clone", t("obImport"), t("obImportHint")],
    ["use", t("obUse"), t("obUseHint")],
  ]
  const refs = React.useRef<(HTMLButtonElement | null)[]>([])

  return (
    <div className="flex min-h-svh flex-col">
      <header className="flex h-[54px] shrink-0 items-center gap-2.5 border-b-hd px-6" data-drag-region>
        <span className="font-heading fs-brand leading-none font-semibold text-ink">skilldots</span>
        <span className="flex-1" />
        <StyleSwitch className="h-[26px] p-0.5" />
      </header>
      <main className="mx-auto w-full max-w-[640px] flex-1 px-6 pt-12 pb-10">
        <h1 className="font-heading fs-h1 leading-tight font-semibold tracking-[-0.01em] text-ink">{t("welcome")}</h1>
        <p className="mt-3 fs-body leading-relaxed text-pretty text-mut">{t("welcomeBody")}</p>

        <div role="radiogroup" aria-label={t("librarySection")} className="mt-7 [border-top:var(--tblHd)]">
          {options.map(([m, title, hint], i) => (
            <button
              key={m}
              ref={(el) => {
                refs.current[i] = el
              }}
              type="button"
              role="radio"
              aria-checked={mode === m}
              tabIndex={mode === m ? 0 : -1}
              onClick={() => setMode(m)}
              onKeyDown={(e) => {
                if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return
                e.preventDefault()
                const j = (i + (e.key === "ArrowDown" ? 1 : options.length - 1)) % options.length
                setMode(options[j][0])
                refs.current[j]?.focus()
              }}
              className={cn("flex w-full items-center gap-3.5 border-b-row px-2.5 py-3.5 text-left hover:bg-row-hv", mode === m && "bg-row-on shadow-row")}
            >
              <span aria-hidden className={cn("grid size-4 flex-none place-items-center rounded-full border-[1.5px]", mode === m ? "border-hue bg-hue" : "border-ln")}>
                {mode === m && <span className="size-1.5 rounded-full bg-white" />}
              </span>
              <span className="min-w-0">
                <span className="block font-heading fs-name font-semibold text-ink">{title}</span>
                <span className="mt-0.5 block fs-meta text-mut">{hint}</span>
              </span>
            </button>
          ))}
        </div>

        <div className="mt-6">
          <LibraryForm key={mode} mode={mode} autoFocus={false} onDone={() => navigate(mode === "init" ? "scan" : "skills")} />
        </div>

        {status && status.unmanaged > 0 && <p className="mt-6 fs-meta text-mut">{t("obFound", { n: status.unmanaged })}</p>}
      </main>
    </div>
  )
}
