import { RotateCwIcon } from "lucide-react"
import { cn } from "cn"

/*
 * shadcn spinner (base-nova). The glyph is Lucide's rotate-cw instead of the
 * default loader: the design draws "working" as the circular arrow. The spin
 * is a CSS animation, so the reduced-motion rule in index.css stops it.
 * Next to a text label pass aria-hidden; on its own it is a status.
 */
function Spinner({ className, ...props }: React.ComponentProps<"svg">) {
  return (
    <RotateCwIcon
      data-slot="spinner"
      role="status"
      aria-label="Loading"
      strokeWidth={1.8}
      className={cn("size-[13px] shrink-0 animate-spin [animation-duration:0.9s]", className)}
      {...props}
    />
  )
}

export { Spinner }
