import { Toggle as TogglePrimitive } from "@base-ui/react/toggle"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"

/*
 * shadcn toggle (Base UI). Two extra looks from the design: "seg" is an item
 * of a segmented control (.skd-seg-item: style switch, view switch, light and
 * dark), "chip" a pill in the filter bar (.skd-chip). Base UI keeps
 * aria-pressed and the roving focus of the group.
 */
const toggleVariants = cva(
  "group/toggle inline-flex items-center justify-center gap-1.5 font-body whitespace-nowrap disabled:pointer-events-none disabled:opacity-45 aria-disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-[13px]",
  {
    variants: {
      variant: {
        default: "rounded-ctl text-mut hover:bg-nav hover:text-ink aria-pressed:bg-nav aria-pressed:text-ink",
        outline: "rounded-ctl border-line text-mut hover:bg-nav hover:text-ink aria-pressed:bg-nav aria-pressed:text-ink",
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

function Toggle({
  className,
  variant = "default",
  size = "default",
  ...props
}: TogglePrimitive.Props & VariantProps<typeof toggleVariants>) {
  return <TogglePrimitive data-slot="toggle" className={cn(toggleVariants({ variant, size, className }))} {...props} />
}

export { Toggle, toggleVariants }
