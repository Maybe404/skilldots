import { useReducedMotion } from "motion/react"
import { ChevronRightIcon } from "lucide-react"
import { cn } from "cn"

import {
  Collapsible as CollapsiblePrimitive,
  CollapsiblePanel as CollapsiblePanelPrimitive,
  CollapsibleTrigger as CollapsibleTriggerPrimitive,
  type CollapsiblePanelProps,
  type CollapsibleProps,
  type CollapsibleTriggerProps,
} from "@/components/animate-ui/primitives/base/collapsible"

/*
 * Animate UI's Base UI collapsible (primitives-base-collapsible): Base UI keeps
 * aria-expanded and the panel link; Animate UI opens the panel to its height
 * with a soft mask wipe. The height is not a transform, so reduced motion is
 * handled here: the panel then opens at once.
 */
function Collapsible(props: CollapsibleProps) {
  return <CollapsiblePrimitive {...props} />
}

function CollapsibleTrigger(props: CollapsibleTriggerProps) {
  return <CollapsibleTriggerPrimitive {...props} />
}

function CollapsiblePanel({ transition, ...props }: CollapsiblePanelProps) {
  const reduce = useReducedMotion()
  return <CollapsiblePanelPrimitive transition={reduce ? { duration: 0 } : (transition ?? { duration: 0.28, ease: [0.16, 1, 0.3, 1] })} {...props} />
}

/** The disclosure chevron: points right, turns down while open (inside a trigger with aria-expanded). */
function DisclosureChevron({ open, className }: { open: boolean; className?: string }) {
  return <ChevronRightIcon aria-hidden className={cn("size-3.5 shrink-0 text-mut transition-transform duration-200", open && "rotate-90", className)} />
}

export { Collapsible, CollapsibleTrigger, CollapsiblePanel, DisclosureChevron }
