import { toast } from "sonner"
import { useApp } from "@/lib/app-state"
import { agentLabel } from "@/lib/agents"
import { backend, type Ref } from "@/lib/backend"

/** Skill operations shared by the Skill page, the detail panel and the palette. */
export function useSkillActions() {
  const { t, agents, run, navigate } = useApp()
  const label = (name: string) => {
    const a = agents.find((x) => x.name === name)
    return a ? agentLabel(a) : name
  }

  const reveal = async (ref: Ref) => {
    if ((await run(() => backend.reveal(ref))).ok) toast(t("revealed"))
  }
  const openInEditor = async (ref: Ref) => {
    if ((await run(() => backend.openInEditor(ref))).ok) toast(t("openedInEditor"))
  }
  const trashToast = (name: string) =>
    toast(t("movedToTrash", { name }), { action: { label: t("openTrash"), onClick: () => navigate("trash") } })

  return { reveal, openInEditor, trashToast, agentLabel: label }
}
