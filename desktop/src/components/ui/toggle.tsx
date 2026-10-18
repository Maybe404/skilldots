import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"

import {
  Toggle as TogglePrimitive,
  ToggleHighlight as ToggleHighlightPrimitive,
  ToggleItem as ToggleItemPrimitive,
  type ToggleProps as TogglePrimitiveProps,
} from "@/components/animate-ui/primitives/base/toggle"

/*
 * Animate UI's Base UI toggle (components-base-toggle): Base UI keeps
 * aria-pressed and the roving focus of a group; Animate UI adds the press
 * scale and fades the pressed background in and out. Two extra looks from the
 * design: "seg" is an item of a segmented control (.skd-seg-item: style
 * switch, view switch, light and dark), "chip" a pill in the filter bar
 * (.skd-chip). The pressed background is the highlight (toggleThumb), so the
 * item itself only changes its text.
 */
const toggleVariants = cva(
  "group/toggle inline-flex items-center justify-center gap-1.5 font-body whitespace-nowrap disabled:pointer-events-none disabled:opacity-45 aria-disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-[13px]",
  {
    variants: {
      variant: {
        default: "rounded-ctl text-mut hover:text-ink data-pressed:text-ink",
        outline: "rounded-ctl border-line text-mut hover:text-ink data-pressed:text-ink",
        seg: "skd-seg-item",
        chip: "skd-chip",
      },
      size: {
        default: "h-7 min-w-7 px-2.5 fs-body",
        sm: "h-[26px] min-w-[26px] px-2 fs-meta",
        none: "",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
)

type Variant = NonNullable<VariantProps<typeof toggleVariants>["variant"]>

/** Corner of each look: the pressed background and the focus ring follow it. */
const toggleRadius: Record<Variant, string> = {
  default: "rounded-ctl",
  outline: "rounded-ctl",
  seg: "rounded-seg",
  chip: "rounded-[13px]",
}

/** The pressed background of each look (in a group it slides between the items). */
const toggleThumb: Record<Variant, string> = {
  default: "rounded-ctl bg-nav",
  outline: "rounded-ctl bg-nav",
  seg: "rounded-seg bg-nav-on shadow-seg",
  chip: "rounded-[13px] bg-ink",
}

type ToggleProps = Omit<TogglePrimitiveProps, "children"> &
  VariantProps<typeof toggleVariants> & {
    children?: React.ReactNode
  }

function Toggle({ className, variant = "default", size = "default", children, ...props }: ToggleProps) {
  const v = variant ?? "default"
  return (
    <TogglePrimitive data-slot="toggle" className={cn("relative inline-flex", toggleRadius[v])} {...props}>
      {/* aria-pressed belongs to the button (Base UI), not to these two layers. */}
      <ToggleHighlightPrimitive aria-pressed={undefined} className={toggleThumb[v]} transition={{ duration: 0.15 }} />
      <ToggleItemPrimitive aria-pressed={undefined} className={cn(toggleVariants({ variant, size }), className)}>
        {children}
      </ToggleItemPrimitive>
    </TogglePrimitive>
  )
}

export { Toggle, toggleRadius, toggleThumb, toggleVariants }
