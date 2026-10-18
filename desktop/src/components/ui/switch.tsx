"use client"

import { Switch as SwitchPrimitive } from "@base-ui/react/switch"
import { cn } from "cn"

/** shadcn switch (Base UI): the design's 34×20 track with a 16px knob. */
function Switch({ className, ...props }: SwitchPrimitive.Root.Props) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn(
        "peer group/switch relative inline-flex h-5 w-[34px] shrink-0 items-center rounded-[10px] outline-none transition-colors after:absolute after:-inset-x-2 after:-inset-y-2 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring data-checked:bg-hue data-unchecked:bg-ln data-disabled:cursor-not-allowed data-disabled:opacity-45",
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        data-slot="switch-thumb"
        className="pointer-events-none absolute top-0.5 left-0.5 block size-4 rounded-full bg-knob shadow-[0_1px_2px_rgba(0,0,0,0.2)] transition-transform duration-150 data-checked:translate-x-[14px]"
      />
    </SwitchPrimitive.Root>
  )
}

export { Switch }
