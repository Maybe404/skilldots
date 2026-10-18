import * as React from "react"
import { Dialog as SheetPrimitive } from "@base-ui/react/dialog"
import { cn } from "cn"
import { XIcon } from "lucide-react"

import {
  Dialog as DialogAnimated,
  DialogBackdrop as DialogBackdropAnimated,
  DialogPopup as DialogPopupAnimated,
  DialogPortal as DialogPortalAnimated,
  type DialogPopupProps,
  type DialogProps,
} from "@/components/animate-ui/primitives/base/dialog"
import { Button } from "@/components/ui/button"

/*
 * shadcn's sheet API on Animate UI's Base UI dialog primitive (Animate UI has
 * no Base UI sheet; its Radix sheet slides the panel in on a spring, which is
 * what this does with the Base UI dialog it already uses for ui/dialog). Base
 * UI keeps the focus trap, Esc and focus return; the panel stays mounted until
 * it has slid out. Reduced motion: the slide is dropped, the fade stays.
 */
type Side = "top" | "right" | "bottom" | "left"

const offscreen: Record<Side, { x?: string; y?: string }> = {
  right: { x: "100%" },
  left: { x: "-100%" },
  top: { y: "-100%" },
  bottom: { y: "100%" },
}

function Sheet(props: DialogProps) {
  return <DialogAnimated {...props} />
}

function SheetTrigger({ ...props }: SheetPrimitive.Trigger.Props) {
  return <SheetPrimitive.Trigger data-slot="sheet-trigger" {...props} />
}

function SheetClose({ ...props }: SheetPrimitive.Close.Props) {
  return <SheetPrimitive.Close data-slot="sheet-close" {...props} />
}

function SheetContent({
  className,
  children,
  side = "right",
  showCloseButton = true,
  closeLabel = "Close",
  ...props
}: DialogPopupProps & {
  side?: Side
  showCloseButton?: boolean
  closeLabel?: string
}) {
  const away = { ...offscreen[side], opacity: 0 }
  return (
    <DialogPortalAnimated>
      <DialogBackdropAnimated data-slot="sheet-overlay" className="fixed inset-0 z-50 bg-[var(--scrim)] opacity-60" animate={{ opacity: 0.6, filter: "blur(0px)" }} />
      <DialogPopupAnimated
        data-slot="sheet-content"
        data-side={side}
        initial={away}
        animate={{ x: 0, y: 0, opacity: 1 }}
        exit={away}
        transition={{ type: "spring", stiffness: 380, damping: 38 }}
        className={cn(
          "fixed z-50 flex flex-col gap-0 bg-dialog font-body fs-body text-ink shadow-pop data-[side=bottom]:inset-x-0 data-[side=bottom]:bottom-0 data-[side=bottom]:h-auto data-[side=bottom]:border-t data-[side=left]:inset-y-0 data-[side=left]:left-0 data-[side=left]:h-full data-[side=left]:w-3/4 data-[side=left]:max-w-[420px] data-[side=left]:border-r data-[side=right]:inset-y-0 data-[side=right]:right-0 data-[side=right]:h-full data-[side=right]:w-3/4 data-[side=right]:max-w-[420px] data-[side=right]:[border-left:var(--cardBd)] data-[side=top]:inset-x-0 data-[side=top]:top-0 data-[side=top]:h-auto data-[side=top]:border-b",
          className,
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <SheetPrimitive.Close data-slot="sheet-close" render={<Button variant="ghost" className="absolute top-3 right-3" size="icon-sm" />}>
            <XIcon />
            <span className="sr-only">{closeLabel}</span>
          </SheetPrimitive.Close>
        )}
      </DialogPopupAnimated>
    </DialogPortalAnimated>
  )
}

function SheetHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="sheet-header" className={cn("flex flex-col gap-1 px-6 pt-[22px] pb-4", className)} {...props} />
}

function SheetFooter({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="sheet-footer" className={cn("mt-auto flex flex-col gap-2 px-6 py-4", className)} {...props} />
}

function SheetTitle({ className, ...props }: SheetPrimitive.Title.Props) {
  return <SheetPrimitive.Title data-slot="sheet-title" className={cn("pr-8 font-heading fs-h3 leading-tight font-bold text-ink", className)} {...props} />
}

function SheetDescription({ className, ...props }: SheetPrimitive.Description.Props) {
  return <SheetPrimitive.Description data-slot="sheet-description" className={cn("fs-meta text-mut", className)} {...props} />
}

export { Sheet, SheetTrigger, SheetClose, SheetContent, SheetHeader, SheetFooter, SheetTitle, SheetDescription }
