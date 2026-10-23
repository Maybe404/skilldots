import * as React from "react"
import { toast } from "sonner"
import { ArrowDownUp } from "lucide-react"

import { errorText, useApp } from "@/lib/app-state"
import { tildify } from "@/lib/agents"
import { backend, SYNC_REF, type CliStatus, type Config, type Theme } from "@/lib/backend"
import { ago } from "@/lib/i18n"
import { ConflictMark, Page } from "@/components/app/bits"
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
  const { t, config, setConfig, status, openDialog, syncLibrary, syncShelved, openResolve, run } = useApp()
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
    await syncLibrary()
    setSyncing(false)
  }

  return (
    <Page title={t("settingsTitle")} summary={t("settingsShared")}>
      <div className="grid grid-cols-2 items-start gap-x-12 max-[900px]:grid-cols-1">
        <div>
          <Section title={t("appearance")} first>
            <Row label={t("styleLabel")}>
              <StyleSwitch className="h-[34px]" />
            </Row>
            {/* Light only until the dark variant exists: 深色 is shown, can't be chosen, and says so. */}
            <Row label={t("mode")} hint={t("darkPending")}>
              <ToggleGroup
                aria-label={t("mode")}
                variant="seg"
                size="none"
                value={[config.theme]}
                onValueChange={(v) => {
                  const next = v[0] as Theme | undefined
                  if (next && next !== "dark") void setConfig({ theme: next })
                }}
                className="skd-seg h-[34px]"
                spacing={2}
              >
                <ToggleGroupItem value="system" className="h-7">{t("themeSystem")}</ToggleGroupItem>
                <ToggleGroupItem value="light" className="h-7">{t("themeLight")}</ToggleGroupItem>
                <ToggleGroupItem value="dark" className="h-7 disabled:opacity-45" disabled aria-describedby="dark-pending">
                  {t("themeDark")}
                </ToggleGroupItem>
              </ToggleGroup>
              <span id="dark-pending" className="sr-only">
                {t("darkPending")}
              </span>
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
              <div className="mt-[3px] font-mono fs-meta break-all text-ink">{tildify(status?.root ?? "")}</div>
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

          <AgentAccess />

          <Section title={t("backupSection")}>
            {/* The remote keeps a whole line when the column is narrow: the button wraps below it. */}
            <div className="flex flex-wrap items-center gap-x-3.5 gap-y-2 border-b-row py-3">
              <div className="min-w-[16rem] flex-1">
                {status?.remote ? (
                  <div className="font-mono fs-meta break-all text-ink">{status.remote}</div>
                ) : (
                  <div className="fs-body text-ink">{t("noRemote")}</div>
                )}
                <div className="mt-[3px] fs-meta text-mut">
                  {status?.changes ? t("unsynced", { n: status.changes }) : t("synced")}
                  {status?.lastSync && ` · ${t("lastSync", { ago: ago(status.lastSync, t) })}`}
                </div>
              </div>
              {/* Syncing also pulls, so it stays offered when nothing here changed. */}
              <Button onClick={() => void backup()} disabled={syncing || !!status?.syncing} variant={status?.changes || !status?.remote ? "default" : "secondary"} className="px-3">
                {syncing ? <Spinner /> : <ArrowDownUp aria-hidden strokeWidth={1.8} />}
                {syncing ? t("syncing") : t("syncNow")}
              </Button>
            </div>
            {(status?.syncing || !!syncShelved?.length) && (
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b-row py-2.5 fs-meta text-ink" role="status">
                <ConflictMark />
                {status?.syncing ? t("syncResolvingBar") : t("syncShelvedToast", { n: syncShelved?.length ?? 0 })}
                <Button variant="link" size="inline" className="fs-meta" onClick={() => openResolve(SYNC_REF)}>
                  {t("resolveConflicts")}
                </Button>
              </div>
            )}
            <ZipBackup />
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

/** The library as one zip file in a folder the person picks, and the way back from one: no git remote needed. */
function ZipBackup() {
  const { t, run, openDialog } = useApp()
  const [busy, setBusy] = React.useState(false)
  const backupTo = async () => {
    const dir = await run(() => backend.pickFolder())
    if (!dir.ok || !dir.value) return
    setBusy(true)
    const r = await run(() => backend.backupZip(dir.value as string))
    setBusy(false)
    if (r.ok) toast(t("backupDone", { n: r.value.skills, path: tildify(r.value.path) }), { action: { label: t("revealInFinder"), onClick: () => void backend.revealBackup(r.value.path) } })
  }
  return (
    <Row label={t("zipBackupLabel")} hint={t("zipBackupHint")}>
      <div className="flex flex-col items-end gap-1.5">
        <Button variant="outline" disabled={busy} onClick={() => void backupTo()}>
          {busy && <Spinner />}
          {busy ? t("backingUp") : t("backupTo")}
        </Button>
        <Button variant="link" size="inline" className="fs-meta" onClick={() => openDialog({ kind: "library", mode: "restore" })}>
          {t("restoreFrom")}
        </Button>
      </div>
    </Row>
  )
}

/** What lets the agents on this Mac use skilldots: the skd command and the skilldots skill. */
function AgentAccess() {
  const { t, status, runTask, reload } = useApp()
  const [cli, setCli] = React.useState<CliStatus | null>(null)
  const [busy, setBusy] = React.useState(false)
  React.useEffect(() => {
    backend.cliStatus().then(setCli, () => setCli(null))
  }, [])

  const cliAction = async (fn: () => Promise<CliStatus>, done: string) => {
    setBusy(true)
    try {
      setCli(await fn())
      toast(done)
    } catch (e) {
      const msg = errorText(t, e)
      if (!msg.includes("cancelled")) toast.error(t("cliFailed", { error: msg }))
    } finally {
      setBusy(false)
    }
  }
  const installSkill = async () => {
    const r = await runTask(t("operating"), () => backend.installSelfSkill())
    if (!r.ok) return
    await reload()
    toast(t("selfSkillToast"))
  }

  const hint = !cli
    ? undefined
    : !cli.canInstall && cli.state !== "installed"
      ? cli.reason === "move"
        ? t("cliMoveHint")
        : t("cliDevHint")
      : cli.state === "installed"
        ? t("cliInstalledHint", { path: cli.path })
        : cli.state === "stale"
          ? t("cliStaleHint", { path: cli.path })
          : cli.state === "other"
            ? t("cliOtherHint", { path: cli.path })
            : t("cliMissingHint", { path: cli.path })

  return (
    <Section title={t("agentAccessSection")}>
      <Row label={t("cliLabel")} hint={hint}>
        {cli && cli.state === "installed" ? (
          <Button variant="outline" disabled={busy} onClick={() => void cliAction(() => backend.removeCli(), t("cliRemovedToast"))}>
            {busy && <Spinner />}
            {t("cliRemove")}
          </Button>
        ) : cli && cli.canInstall && cli.state !== "other" ? (
          <Button disabled={busy} onClick={() => void cliAction(() => backend.installCli(), t("cliInstalledToast"))}>
            {busy && <Spinner />}
            {cli.state === "stale" ? t("cliRepair") : t("cliInstall")}
          </Button>
        ) : null}
      </Row>
      <Row label={t("selfSkillLabel")} hint={t("selfSkillHint")}>
        {status?.selfSkill ? (
          <span className="fs-meta text-mut">{t("selfSkillInstalled")}</span>
        ) : (
          <Button variant="outline" onClick={() => void installSkill()}>
            {t("selfSkillInstall")}
          </Button>
        )}
      </Row>
    </Section>
  )
}
