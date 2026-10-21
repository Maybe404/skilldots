import * as React from "react"

import { cn } from "@/lib/utils"
import { useApp } from "@/lib/app-state"
import { LibraryForm } from "@/components/dialogs/ops-dialogs"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"

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

  return (
    <div className="flex min-h-svh flex-col">
      <header className="skd-titlebar flex h-[55px] shrink-0 items-center gap-2.5 border-b-hd pr-6">
        <span className="font-heading fs-brand leading-none font-semibold text-ink">skilldots</span>
      </header>
      <main className="mx-auto w-full max-w-[640px] flex-1 px-6 pt-12 pb-10">
        <h1 className="font-heading fs-h1 leading-tight font-semibold tracking-[-0.01em] text-ink">{t("welcome")}</h1>
        <p className="mt-3 fs-body leading-relaxed text-pretty text-mut">{t("welcomeBody")}</p>

        <RadioGroup value={mode} onValueChange={(v) => setMode(v as Mode)} aria-label={t("librarySection")} className="mt-7 gap-0 [border-top:var(--tblHd)]">
          {options.map(([m, title, hint]) => (
            <label
              key={m}
              className={cn("flex w-full cursor-pointer items-center gap-3.5 border-b-row px-2.5 py-3.5 text-left hover:bg-row-hv", mode === m && "bg-row-on shadow-row")}
            >
              <RadioGroupItem value={m} />
              <span className="min-w-0">
                <span className="block font-heading fs-name font-semibold text-ink">{title}</span>
                <span className="mt-0.5 block fs-meta text-mut">{hint}</span>
              </span>
            </label>
          ))}
        </RadioGroup>

        <div className="mt-6">
          <LibraryForm key={mode} mode={mode} autoFocus={false} onDone={() => navigate(mode === "init" ? "scan" : "skills")} />
        </div>

        {status && status.unmanaged > 0 && <p className="mt-6 fs-meta text-mut">{t("obFound", { n: status.unmanaged })}</p>}
      </main>
    </div>
  )
}
