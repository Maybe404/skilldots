"use client"

import { useReducedMotion } from "motion/react"
import { cn } from "cn"

import { Switch as SwitchPrimitive, SwitchThumb as SwitchThumbPrimitive, type SwitchProps as SwitchPrimitiveProps } from "@/components/animate-ui/primitives/base/switch"

/*
 * Animate UI's Base UI switch (components-base-switch) drawn as the design's
 * 34×20 track with a 16px knob: the knob springs across and stretches while
 * pressed. Colours are the install hue (on) and the line (off). With reduced
 * motion the knob moves without the spring or the stretch.
 */
function Switch({ className, ...props }: SwitchPrimitiveProps) {
  const reduce = useReducedMotion()
  return (
    <SwitchPrimitive
      nativeButton
      className={cn(
        "peer relative flex h-5 w-[34px] shrink-0 items-center justify-start rounded-[10px] px-0.5 transition-colors after:absolute after:-inset-x-2 after:-inset-y-2 data-checked:justify-end data-checked:bg-hue data-unchecked:bg-ln data-disabled:cursor-not-allowed data-disabled:opacity-45",
        className,
      )}
      {...props}
    >
      <SwitchThumbPrimitive
        className="pointer-events-none relative z-10 block size-4 rounded-full bg-knob shadow-[var(--knobSh)]"
        pressedAnimation={reduce ? undefined : { width: 20 }}
        transition={reduce ? { duration: 0 } : undefined}
      />
    </SwitchPrimitive>
  )
}

export { Switch }
