import * as React from "react"
import { toast } from "sonner"
import { CloudUpload } from "lucide-react"

import { useApp } from "@/lib/app-state"
import { backend, type Config, type Theme } from "@/lib/backend"
import { ago } from "@/lib/i18n"
import { Page } from "@/components/app/bits"
import { StyleSwitch } from "@/components/app/header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

function Section({ title, children, first }: { title: string; children: React.ReactNode; first?: boolean }) {
  return (
    <section className={first ? undefined : "mt-[26px]"}>
      <h2 className="flex h-[31px] items-center border-b-head fs-meta font-normal text-mut">{title}</h2>
      {children}
    </section>
  )
}

function Row({ label, hint, htmlFor, children }: { label: string; hint?: string; htmlFor?: string; children: React.ReactNode }) {
  return (
    <div className="flex min-h-[55px] items-center justify-between gap-4 border-b-row py-2">
      <div className="min-w-0">
        <label htmlFor={htmlFor} className="text-ink">
          {label}
        </label>
        {hint && <p className="mt-px fs-meta text-mut">{hint}</p>}
      </div>
      <div className="flex shrink-0 justify-end">{children}</div>
    </div>
  )
}

/** Settings shared with the CLI, plus the desktop's own style switch. */
export function SettingsPage() {
  const { t, config, setConfig, status, openDialog, run, runTask, reload } = useApp()
  const editors: [Config["editor"], string][] = [
    ["auto", t("editorAuto")],
    ["code", "VS Code"],
    ["cursor", "Cursor"],
    ["zed", "Zed"],
    ["custom", t("editorCustom")],
  ]
  const [syncing, setSyncing] = React.useState(false)

  const backup = async () => {
    if (!status?.remote) {
      openDialog({ kind: "sync" })
      return
    }
    setSyncing(true)
    const r = await runTask(t("syncing"), () => backend.sync({}))
    setSyncing(false)
    if (!r.ok) return
    await reload()
    toast(r.value.committed ? t("syncedToast", { remote: r.value.remote }) : t("nothingToSync"))
  }

  return (
    <Page title={t("settingsTitle")} summary={t("settingsShared")}>
      <div className="grid grid-cols-2 items-start gap-x-12 max-[900px]:grid-cols-1">
        <div>
          <Section title={t("appearance")} first>
            <Row label={t("styleLabel")}>
              <StyleSwitch className="h-[34px]" />
            </Row>
            <Row label={t("mode")} hint={config.theme === "dark" ? t("darkPending") : undefined}>
              <ToggleGroup
                aria-label={t("mode")}
                variant="seg"
                size="none"
                value={[config.theme]}
                onValueChange={(v) => {
                  const next = v[0] as Theme | undefined
                  if (!next) return
                  if (next === "dark") {
                    toast(t("darkPending"))
                    return
                  }
                  void setConfig({ theme: next })
                }}
                className="skd-seg h-[34px] items-start"
                spacing={2}
              >
                <ToggleGroupItem value="system">{t("themeSystem")}</ToggleGroupItem>
                <ToggleGroupItem value="light">{t("themeLight")}</ToggleGroupItem>
                <ToggleGroupItem value="dark" aria-disabled title={t("darkPending")}>
                  {t("themeDark")}
                </ToggleGroupItem>
              </ToggleGroup>
            </Row>
            <Row label={t("language")}>
              <Select value={config.lang} onValueChange={(v) => v && setConfig({ lang: v as Config["lang"] })} items={{ zh: "简体中文", en: "English" }}>
                <SelectTrigger aria-label={t("language")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="zh">简体中文</SelectItem>
                  <SelectItem value="en">English</SelectItem>
                </SelectContent>
              </Select>
            </Row>
          </Section>

          <Section title={t("editorSection")}>
            <Row label={t("editorLabel")}>
              <Select value={config.editor} onValueChange={(v) => v && setConfig({ editor: v as Config["editor"] })} items={Object.fromEntries(editors)}>
                <SelectTrigger aria-label={t("editorLabel")} className="max-w-[300px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {editors.map(([v, label]) => (
                    <SelectItem key={v} value={v}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Row>
            {config.editor === "custom" && (
              <div className="border-b-row py-3">
                <label htmlFor="editor-cmd" className="fs-meta text-mut">
                  {t("editorCommand")}
                </label>
                <Input
                  id="editor-cmd"
                  defaultValue={config.editorCommand}
                  onBlur={(e) => setConfig({ editorCommand: e.target.value })}
                  placeholder="subl {path}"
                  className="mt-1 font-mono fs-meta"
                  spellCheck={false}
                />
                <p className="mt-1 fs-meta text-mut">{t("editorCommandHint", { path: "{path}" })}</p>
              </div>
            )}
            <Row label={t("checkOnLaunch")} hint={t("checkOnLaunchHint")} htmlFor="check-launch">
              <Switch id="check-launch" checked={config.checkOnLaunch} onCheckedChange={(v) => setConfig({ checkOnLaunch: v })} />
            </Row>
          </Section>
        </div>

        <div className="max-[900px]:mt-[26px]">
          <Section title={t("librarySection")} first>
            <div className="border-b-row py-3">
              <div className="fs-meta text-mut">{t("libraryPath")}</div>
              <div className="mt-[3px] font-mono fs-meta break-all text-ink">{status?.root}</div>
              <div className="mt-2.5 flex flex-wrap gap-x-3.5 gap-y-1.5 fs-meta">
                <Button variant="link" size="inline" onClick={async () => (await run(() => backend.revealLibrary())).ok && toast(t("revealed"))}>
                  {t("revealInFinder")}
                </Button>
                <Button variant="link" size="inline" onClick={() => openDialog({ kind: "library", mode: "use" })}>
                  {t("switchLibrary")}
                </Button>
                <Button variant="link" size="inline" onClick={() => openDialog({ kind: "library", mode: "clone" })}>
                  {t("importLibrary")}
                </Button>
                <Button variant="link" size="inline" onClick={() => openDialog({ kind: "library", mode: "init" })}>
                  {t("newLibrary")}
                </Button>
              </div>
            </div>
          </Section>

          <Section title={t("backupSection")}>
            <div className="flex items-center gap-3.5 border-b-row py-3">
              <div className="min-w-0 flex-1">
                {status?.remote ? (
                  <div className="truncate font-mono fs-meta text-ink">{status.remote}</div>
                ) : (
                  <div className="fs-body text-ink">{t("noRemote")}</div>
                )}
                <div className="mt-[3px] fs-meta text-mut">
                  {status?.changes ? t("unsynced", { n: status.changes }) : t("synced")}
                  {status?.lastSync && ` · ${t("lastSync", { ago: ago(status.lastSync, t) })}`}
                </div>
              </div>
              <Button onClick={() => void backup()} disabled={syncing} variant={status?.changes || !status?.remote ? "default" : "secondary"} className="px-3">
                {syncing ? <Spinner /> : <CloudUpload aria-hidden strokeWidth={1.8} />}
                {syncing ? t("syncing") : status?.changes || !status?.remote ? t("syncNow") : t("backedUp")}
              </Button>
            </div>
            <p className="py-3 fs-meta text-mut">
              {t("backupNote2")}{" "}
              <Button variant="link" size="inline" onClick={() => openDialog({ kind: "sync" })}>
                {t("backupWithMessage")}
              </Button>
            </p>
          </Section>
        </div>
      </div>
    </Page>
  )
}
