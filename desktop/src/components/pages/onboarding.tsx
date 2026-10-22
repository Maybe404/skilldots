import * as React from "react"
import { toast } from "sonner"

import { cn } from "@/lib/utils"
import { useApp } from "@/lib/app-state"
import { backend } from "@/lib/backend"
import { LibraryForm } from "@/components/dialogs/ops-dialogs"
import { RefreshIcon } from "@/components/app/bits"
import { Button } from "@/components/ui/button"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"

type Mode = "init" | "clone" | "use"

/**
 * First run (not drawn in the design): the top bar's brand and style switch,
 * a page title like the other pages, and the three ways to get a library as
 * radio rows in the design's table style. A new library is followed by one
 * step that can be skipped: your own skills repository, added as yours.
 * onDone: first run is over (the app shows its pages).
 */
export function Onboarding({ onDone }: { onDone: () => void }) {
  const { t, status, navigate } = useApp()
  const [mode, setMode] = React.useState<Mode>("init")
  const [step, setStep] = React.useState<"library" | "own">("library")
  const options: [Mode, string, string][] = [
    ["init", t("obNew"), t("obNewHint")],
    ["clone", t("obImport"), t("obImportHint")],
    ["use", t("obUse"), t("obUseHint")],
  ]
  const finish = (view: "scan" | "skills") => {
    navigate(view)
    onDone()
  }

  return (
    <div className="flex min-h-svh flex-col">
      <header className="skd-titlebar flex h-[55px] shrink-0 items-center gap-2.5 border-b-hd pr-6">
        <span className="font-heading fs-brand leading-none font-semibold text-ink">skilldots</span>
      </header>
      <main className="mx-auto w-full max-w-[640px] flex-1 px-6 pt-12 pb-10">
        {step === "own" ? (
          <OwnRepoStep onDone={() => finish("scan")} />
        ) : (
          <>
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
              <LibraryForm key={mode} mode={mode} autoFocus={false} onDone={() => (mode === "init" ? setStep("own") : finish("skills"))} />
            </div>

            {status && status.unmanaged > 0 && <p className="mt-6 fs-meta text-mut">{t("obFound", { n: status.unmanaged })}</p>}
          </>
        )}
      </main>
    </div>
  )
}

/** "有自己的 skills 仓库吗？": owner/repo, added as your own repository and followed whole (it may be empty). */
function OwnRepoStep({ onDone }: { onDone: () => void }) {
  const { t, runTask, reload, agents } = useApp()
  const [input, setInput] = React.useState("")
  const [busy, setBusy] = React.useState(false)

  const add = async () => {
    setBusy(true)
    const r = await runTask(t("operating"), async () => {
      const src = await backend.parseSource(input.trim())
      const preview = await backend.show(src)
      const targets = agents.filter((a) => a.isDefault && a.enabled).map((a) => a.name)
      return backend.add(preview, { skills: [], categories: [], all: true, agents: targets, own: true })
    })
    setBusy(false)
    if (!r.ok) return
    await reload().catch(() => undefined)
    toast(t("ownAddedToast", { repo: r.value.repo }))
    onDone()
  }

  return (
    <>
      <h1 className="font-heading fs-h1 leading-tight font-semibold tracking-[-0.01em] text-ink">{t("obOwnTitle")}</h1>
      <p className="mt-3 fs-body leading-relaxed text-pretty text-mut">{t("obOwnBody")}</p>
      <form
        className="mt-7 grid gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (input.trim()) void add()
        }}
      >
        <Field>
          <FieldLabel htmlFor="ob-own">{t("obOwnLabel")}</FieldLabel>
          <Input id="ob-own" autoFocus value={input} onChange={(e) => setInput(e.target.value)} placeholder="owner/repo" className="font-mono fs-body" spellCheck={false} autoComplete="off" />
        </Field>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" size="lg" onClick={onDone} disabled={busy}>
            {t("obSkip")}
          </Button>
          <Button type="submit" size="lg" disabled={!input.trim() || busy}>
            {busy && <RefreshIcon spinning />}
            {t("obOwnAdd")}
          </Button>
        </div>
      </form>
    </>
  )
}
