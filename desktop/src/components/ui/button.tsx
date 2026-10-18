import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"

/*
 * shadcn button (base-nova, Base UI), restyled with the design tokens:
 * buttons in the design are 26–32px tall, use the body font and the control
 * radius (rounded-ctl), and come in a few fills — primary (pri), accent (ac,
 * "应用"), ink (confirm), outline (1.5px line), ghost and link.
 */
const buttonVariants = cva(
  "group/button inline-flex shrink-0 items-center justify-center gap-1.5 font-body whitespace-nowrap transition-[color,background-color,opacity] select-none disabled:pointer-events-none disabled:opacity-45 aria-disabled:opacity-45 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-[13px]",
  {
    variants: {
      variant: {
        default: "bg-pri font-semibold text-pri-fg hover:opacity-90",
        accent: "bg-ac font-semibold text-ac-ink hover:opacity-90",
        ink: "bg-ink font-semibold text-bg hover:opacity-90",
        hue: "bg-hue text-bg hover:opacity-90",
        outline: "box-content border-line text-ink hover:bg-nav aria-expanded:bg-nav",
        secondary: "bg-nav text-ink hover:text-ink",
        ghost: "text-mut hover:bg-nav hover:text-ink aria-expanded:bg-nav aria-expanded:text-ink",
        destructive: "bg-ink font-semibold text-bg hover:opacity-90",
        link: "h-auto px-0 text-hue-ink underline decoration-ln underline-offset-[3px] hover:decoration-current",
        quiet: "h-auto px-0 text-mut underline decoration-ln underline-offset-[3px] hover:text-ink",
      },
      size: {
        default: "h-7 rounded-ctl px-2.5 fs-body",
        xs: "h-6 rounded-ctl px-1.5 fs-meta",
        sm: "h-[26px] rounded-ctl px-2.5 fs-meta",
        lg: "h-8 rounded-ctl px-3.5 fs-body",
        pill: "h-8 rounded-btn pr-3.5 pl-[11px] fs-body",
        icon: "size-7 rounded-ctl",
        "icon-xs": "size-6 rounded-ctl",
        "icon-sm": "size-[26px] rounded-ctl",
        "icon-lg": "size-8 rounded-ctl",
        inline: "h-auto rounded-none p-0 text-[length:inherit]",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
)

function Button({
  className,
  variant = "default",
  size = "default",
  ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
  return <ButtonPrimitive data-slot="button" className={cn(buttonVariants({ variant, size, className }))} {...props} />
}

export { Button, buttonVariants }
