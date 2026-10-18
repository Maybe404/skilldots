import * as React from "react"
import { cn } from "cn"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "flex field-sizing-content min-h-16 w-full rounded-ctl border-line bg-dialog px-2.5 py-2 font-body fs-body text-ink transition-colors placeholder:text-mut focus-visible:border-hue focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring/40 disabled:cursor-not-allowed disabled:opacity-50 read-only:bg-nav aria-invalid:border-ac",
        className,
      )}
      {...props}
    />
  )
}

export { Textarea }
