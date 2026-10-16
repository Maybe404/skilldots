import * as React from "react"
import { Download, FolderOpen, FolderPlus, Layers } from "lucide-react"

import { cn } from "@/lib/utils"
import { useApp } from "@/lib/app-state"
import { LibraryForm } from "@/components/dialogs/ops-dialogs"

type Mode = "init" | "clone" | "use"

export function Onboarding() {
  const { t, status, navigate } = useApp()
  const [mode, setMode] = React.useState<Mode>("init")
  const options: [Mode, React.ReactNode, string, string][] = [
    ["init", <FolderPlus key="i" />, t("obNew"), t("obNewHint")],
    ["clone", <Download key="c" />, t("obImport"), t("obImportHint")],
    ["use", <FolderOpen key="u" />, t("obUse"), t("obUseHint")],
  ]
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <div className="glass w-full max-w-xl rounded-2xl p-8" style={{ boxShadow: "var(--glass-shadow)" }}>
        <span className="flex size-11 items-center justify-center rounded-xl bg-foreground text-background">
          <Layers className="size-5" />
        </span>
        <h1 className="mt-5 text-2xl font-semibold tracking-tight">{t("welcome")}</h1>
        <p className="mt-2 text-[0.9rem] leading-relaxed text-muted-foreground">{t("welcomeBody")}</p>

        <div role="radiogroup" aria-label={t("navLibrary")} className="mt-6 grid gap-2">
          {options.map(([m, icon, title, hint]) => (
            <button
              key={m}
              type="button"
              role="radio"
              aria-checked={mode === m}
              onClick={() => setMode(m)}
              onKeyDown={(e) => {
                const i = options.findIndex(([x]) => x === m)
                if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                  e.preventDefault()
                  const next = options[(i + (e.key === "ArrowDown" ? 1 : options.length - 1)) % options.length][0]
                  setMode(next)
                  ;(e.currentTarget.parentElement?.children[options.findIndex(([x]) => x === next)] as HTMLElement | undefined)?.focus()
                }
              }}
              tabIndex={mode === m ? 0 : -1}
              className={cn(
                "flex items-start gap-3 rounded-xl border bg-background/70 p-3.5 text-left [&_svg]:mt-0.5 [&_svg]:size-4.5 [&_svg]:shrink-0",
                mode === m ? "border-foreground" : "hover:bg-background",
              )}
            >
              {icon}
              <span>
                <span className="block font-medium">{title}</span>
                <span className="block text-[0.82rem] text-muted-foreground">{hint}</span>
              </span>
            </button>
          ))}
        </div>

        <div className="mt-5 border-t pt-5">
          <LibraryForm key={mode} mode={mode} onDone={() => navigate(mode === "init" ? "scan" : "skills")} />
        </div>

        {status && status.unmanaged > 0 && <p className="mt-5 text-[0.8rem] text-muted-foreground">{t("obFound", { n: status.unmanaged })}</p>}
      </div>
    </div>
  )
}
