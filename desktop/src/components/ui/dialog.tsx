"use client"

import * as React from "react"
import { cn } from "cn"
import { XIcon } from "lucide-react"

import {
  Dialog as DialogPrimitive,
  DialogBackdrop as DialogBackdropPrimitive,
  DialogClose as DialogClosePrimitive,
  DialogDescription as DialogDescriptionPrimitive,
  DialogPopup as DialogPopupPrimitive,
  DialogPortal as DialogPortalPrimitive,
  DialogTitle as DialogTitlePrimitive,
  DialogTrigger as DialogTriggerPrimitive,
  type DialogBackdropProps,
  type DialogCloseProps,
  type DialogDescriptionProps,
  type DialogPopupProps,
  type DialogPortalProps,
  type DialogProps,
  type DialogTitleProps,
  type DialogTriggerProps,
} from "@/components/animate-ui/primitives/base/dialog"
import { DialogClosing } from "@/lib/dialog-presence"
import { Button } from "@/components/ui/button"

/*
 * shadcn's dialog API on Animate UI's Base UI dialog (components-base-dialog):
 * Base UI keeps the focus trap, Esc, outside press and focus return; Animate
 * UI tips the box in from above and blurs it in and out, and keeps it mounted
 * until the exit has played. With reduced motion only the fade remains (the
 * app's MotionConfig drops the transform). The box is the design's 490px card
 * and never outgrows the window (it scrolls inside instead).
 */
const popupTransition = { type: "spring", stiffness: 320, damping: 30 } as const

function Dialog(props: DialogProps) {
  const closing = React.useContext(DialogClosing)
  return <DialogPrimitive {...props} {...(closing && { open: false })} />
}

function DialogTrigger(props: DialogTriggerProps) {
  return <DialogTriggerPrimitive {...props} />
}

function DialogPortal(props: DialogPortalProps) {
  return <DialogPortalPrimitive {...props} />
}

function DialogClose(props: DialogCloseProps) {
  return <DialogClosePrimitive {...props} />
}

function DialogOverlay({ className, ...props }: DialogBackdropProps) {
  return <DialogBackdropPrimitive data-slot="dialog-overlay" className={cn("fixed inset-0 isolate z-50 bg-[var(--scrim)] backdrop-blur-[3px]", className)} {...props} />
}

function DialogContent({
  className,
  children,
  showCloseButton = true,
  closeLabel = "Close",
  ...props
}: DialogPopupProps & {
  showCloseButton?: boolean
  closeLabel?: string
}) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPopupPrimitive
        data-slot="dialog-content"
        transition={popupTransition}
        className={cn(
          "fixed top-1/2 left-1/2 z-50 grid max-h-[calc(100svh-2rem)] w-[490px] max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 overflow-y-auto rounded-card border-card bg-dialog px-6 py-[22px] font-body fs-body text-ink shadow-pop outline-none",
          className,
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <DialogClosePrimitive data-slot="dialog-close" render={<Button variant="ghost" className="absolute top-3 right-3" size="icon-sm" />}>
            <XIcon />
            <span className="sr-only">{closeLabel}</span>
          </DialogClosePrimitive>
        )}
      </DialogPopupPrimitive>
    </DialogPortal>
  )
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div data-slot="dialog-header" className={cn("flex flex-col gap-2.5", className)} {...props} />
}

function DialogFooter({
  className,
  showCloseButton = false,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  showCloseButton?: boolean
}) {
  return (
    <div data-slot="dialog-footer" className={cn("mt-1.5 flex flex-wrap items-center justify-end gap-2", className)} {...props}>
      {children}
      {showCloseButton && <DialogClosePrimitive render={<Button variant="outline" />}>Close</DialogClosePrimitive>}
    </div>
  )
}

function DialogTitle({ className, ...props }: DialogTitleProps) {
  return <DialogTitlePrimitive data-slot="dialog-title" className={cn("pr-8 font-heading fs-h3 leading-tight font-bold text-ink", className)} {...props} />
}

function DialogDescription({ className, ...props }: DialogDescriptionProps) {
  return (
    <DialogDescriptionPrimitive
      data-slot="dialog-description"
      className={cn("fs-body leading-[1.55] text-pretty text-mut *:[a]:underline *:[a]:underline-offset-3 *:[a]:hover:text-ink", className)}
      {...props}
    />
  )
}

export { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogOverlay, DialogPortal, DialogTitle, DialogTrigger }
