import * as React from "react"
import { type VariantProps } from "class-variance-authority"
import { cn } from "cn"

import {
  Toggle as TogglePrimitive,
  ToggleGroup as ToggleGroupPrimitive,
  ToggleGroupHighlight as ToggleGroupHighlightPrimitive,
  ToggleHighlight as ToggleHighlightPrimitive,
  type ToggleGroupProps as ToggleGroupPrimitiveProps,
  type ToggleProps as TogglePrimitiveProps,
} from "@/components/animate-ui/primitives/base/toggle-group"
import { toggleRadius, toggleThumb, toggleVariants } from "@/components/ui/toggle"

/*
 * Animate UI's Base UI toggle group (components-base-toggle-group): Base UI
 * keeps the group semantics, aria-pressed and the arrow-key roving focus;
 * Animate UI slides one highlight to the pressed item. The highlight is drawn
 * with the look's tokens (toggleThumb), so the segmented controls keep the
 * design's raised white (clean) or hatched (doodle) thumb.
 */
type Look = VariantProps<typeof toggleVariants>

const ToggleGroupContext = React.createContext<Look>({ variant: "default", size: "default" })

function ToggleGroup({
  className,
  variant = "default",
  size = "default",
  spacing = 2,
  children,
  ...props
}: Omit<ToggleGroupPrimitiveProps, "multiple"> & Look & { spacing?: number }) {
  return (
    <ToggleGroupPrimitive
      data-slot="toggle-group"
      data-variant={variant}
      style={{ "--gap": spacing } as React.CSSProperties}
      className={cn("group/toggle-group flex w-fit flex-row items-center gap-[calc(var(--gap)*1px)]", className)}
      {...props}
    >
      <ToggleGroupContext.Provider value={{ variant, size }}>
        <ToggleGroupHighlightPrimitive className={toggleThumb[variant ?? "default"]} transition={{ type: "spring", stiffness: 420, damping: 36 }}>
          {children}
        </ToggleGroupHighlightPrimitive>
      </ToggleGroupContext.Provider>
    </ToggleGroupPrimitive>
  )
}

function ToggleGroupItem({ className, children, variant, size, value, ...props }: Omit<TogglePrimitiveProps, "value"> & Look & { value: string }) {
  const look = React.useContext(ToggleGroupContext)
  const v = variant ?? look.variant ?? "default"
  return (
    <ToggleHighlightPrimitive value={value} className={cn("flex", toggleRadius[v])}>
      <TogglePrimitive
        data-slot="toggle-group-item"
        value={value}
        whileTap={{ scale: 0.97 }}
        className={cn("shrink-0 focus-visible:z-10", toggleVariants({ variant: v, size: size ?? look.size }), className)}
        {...props}
      >
        {children}
      </TogglePrimitive>
    </ToggleHighlightPrimitive>
  )
}

export { ToggleGroup, ToggleGroupItem }
