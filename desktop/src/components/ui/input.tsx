import * as React from "react"
import { Input as InputPrimitive } from "@base-ui/react/input"
import { cn } from "cn"

/** shadcn input (Base UI) with the design's field look: 1.5px line, control radius. */
function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <InputPrimitive
      type={type}
      data-slot="input"
      className={cn(
        "h-8 w-full min-w-0 rounded-ctl border-line bg-dialog px-2.5 py-1 font-body fs-body text-ink transition-colors outline-none placeholder:text-mut focus-visible:border-hue focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring/40 disabled:pointer-events-none disabled:opacity-50 read-only:bg-nav aria-invalid:border-ac",
        className,
      )}
      {...props}
    />
  )
}

export { Input }
