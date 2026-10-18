import { cn } from "cn"

import {
  Tooltip as TooltipPrimitive,
  TooltipArrow as TooltipArrowPrimitive,
  TooltipPopup as TooltipPopupPrimitive,
  TooltipPortal as TooltipPortalPrimitive,
  TooltipPositioner as TooltipPositionerPrimitive,
  TooltipProvider as TooltipProviderPrimitive,
  TooltipTrigger as TooltipTriggerPrimitive,
  type TooltipPopupProps,
  type TooltipPositionerProps,
  type TooltipProps,
  type TooltipProviderProps,
  type TooltipTriggerProps,
} from "@/components/animate-ui/primitives/base/tooltip"

/*
 * Animate UI's Base UI tooltip (components-base-tooltip): Base UI keeps the
 * hover / focus delays, Esc and the accessible description; Animate UI springs
 * the bubble out of its trigger (opacity only with reduced motion, through
 * the app's MotionConfig). Drawn as the design's ink bubble.
 */
function TooltipProvider({ delay = 0, ...props }: TooltipProviderProps) {
  return <TooltipProviderPrimitive delay={delay} {...props} />
}

function Tooltip(props: TooltipProps) {
  return <TooltipPrimitive {...props} />
}

function TooltipTrigger(props: TooltipTriggerProps) {
  return <TooltipTriggerPrimitive {...props} />
}

function TooltipContent({
  className,
  side = "top",
  sideOffset = 6,
  align = "center",
  alignOffset = 0,
  children,
  ...props
}: TooltipPopupProps & Pick<TooltipPositionerProps, "align" | "alignOffset" | "side" | "sideOffset">) {
  return (
    <TooltipPortalPrimitive>
      <TooltipPositionerPrimitive align={align} alignOffset={alignOffset} side={side} sideOffset={sideOffset} className="isolate z-50">
        <TooltipPopupPrimitive
          data-slot="tooltip-content"
          transition={{ type: "spring", stiffness: 420, damping: 30 }}
          className={cn(
            "z-50 inline-flex w-fit max-w-xs origin-(--transform-origin) items-center gap-1.5 rounded-ctl bg-ink px-2.5 py-1.5 font-body fs-meta leading-snug text-bg has-data-[slot=kbd]:pr-1.5 **:data-[slot=kbd]:relative **:data-[slot=kbd]:isolate **:data-[slot=kbd]:z-50 **:data-[slot=kbd]:rounded-sm",
            className,
          )}
          {...props}
        >
          {children}
          <TooltipArrowPrimitive className="z-50 size-2.5 translate-y-[calc(-50%-2px)] rotate-45 rounded-[2px] bg-ink fill-ink data-[side=bottom]:top-1 data-[side=inline-end]:top-1/2! data-[side=inline-end]:-left-1 data-[side=inline-end]:-translate-y-1/2 data-[side=inline-start]:top-1/2! data-[side=inline-start]:-right-1 data-[side=inline-start]:-translate-y-1/2 data-[side=left]:top-1/2! data-[side=left]:-right-1 data-[side=left]:-translate-y-1/2 data-[side=right]:top-1/2! data-[side=right]:-left-1 data-[side=right]:-translate-y-1/2 data-[side=top]:-bottom-2.5" />
        </TooltipPopupPrimitive>
      </TooltipPositionerPrimitive>
    </TooltipPortalPrimitive>
  )
}

export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider }
