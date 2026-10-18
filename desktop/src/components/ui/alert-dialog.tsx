import * as React from "react"
import { AlertDialog as AlertDialogPrimitive } from "@base-ui/react/alert-dialog"
import { cn } from "cn"

import {
  AlertDialog as AlertDialogAnimated,
  AlertDialogBackdrop as AlertDialogBackdropAnimated,
  AlertDialogPopup as AlertDialogPopupAnimated,
  AlertDialogPortal as AlertDialogPortalAnimated,
  AlertDialogTrigger as AlertDialogTriggerAnimated,
  type AlertDialogBackdropProps,
  type AlertDialogPopupProps,
  type AlertDialogPortalProps,
  type AlertDialogProps,
  type AlertDialogTriggerProps,
} from "@/components/animate-ui/primitives/base/alert-dialog"
import { DialogClosing } from "@/lib/dialog-presence"
import { Button } from "@/components/ui/button"

/*
 * shadcn's alert dialog API on Animate UI's Base UI alert dialog
 * (components-base-alert-dialog), with the same entrance as ui/dialog: Base
 * UI keeps the focus trap and focus return, Animate UI the tip-in and blur,
 * and the exit plays before the box unmounts. Reduced motion: fade only.
 */
function AlertDialog(props: AlertDialogProps) {
  const closing = React.useContext(DialogClosing)
  return <AlertDialogAnimated {...props} {...(closing && { open: false })} />
}

function AlertDialogTrigger(props: AlertDialogTriggerProps) {
  return <AlertDialogTriggerAnimated {...props} />
}

function AlertDialogPortal(props: AlertDialogPortalProps) {
  return <AlertDialogPortalAnimated {...props} />
}

function AlertDialogOverlay({ className, ...props }: AlertDialogBackdropProps) {
  return <AlertDialogBackdropAnimated data-slot="alert-dialog-overlay" className={cn("fixed inset-0 isolate z-50 bg-[var(--scrim)] backdrop-blur-[3px]", className)} {...props} />
}

function AlertDialogContent({
  className,
  size = "default",
  ...props
}: AlertDialogPopupProps & {
  size?: "default" | "sm"
}) {
  return (
    <AlertDialogPortal>
      <AlertDialogOverlay />
      <AlertDialogPopupAnimated
        data-slot="alert-dialog-content"
        data-size={size}
        transition={{ type: "spring", stiffness: 320, damping: 30 }}
        className={cn(
          "group/alert-dialog-content fixed top-1/2 left-1/2 z-50 grid max-h-[calc(100svh-2rem)] w-[490px] max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-0 overflow-y-auto rounded-card border-card bg-dialog px-6 py-[22px] font-body fs-body text-ink shadow-pop outline-none",
          className,
        )}
        {...props}
      />
    </AlertDialogPortal>
  )
}

function AlertDialogHeader({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-dialog-header"
      className={cn(
        "grid gap-2.5 text-left",
        className
      )}
      {...props}
    />
  )
}

function AlertDialogFooter({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-dialog-footer"
      className={cn(
        "mt-[22px] flex flex-wrap items-center justify-end gap-2",
        className
      )}
      {...props}
    />
  )
}

function AlertDialogMedia({
  className,
  ...props
}: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-dialog-media"
      className={cn(
        "mb-2 inline-flex size-10 items-center justify-center rounded-ctl bg-nav *:[svg:not([class*='size-'])]:size-5",
        className
      )}
      {...props}
    />
  )
}

function AlertDialogTitle({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Title>) {
  return (
    <AlertDialogPrimitive.Title
      data-slot="alert-dialog-title"
      className={cn(
        "font-heading fs-h3 leading-tight font-bold text-ink",
        className
      )}
      {...props}
    />
  )
}

function AlertDialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Description>) {
  return (
    <AlertDialogPrimitive.Description
      data-slot="alert-dialog-description"
      className={cn(
        "fs-body leading-[1.55] text-pretty text-mut *:[a]:underline *:[a]:underline-offset-3 *:[a]:hover:text-ink",
        className
      )}
      {...props}
    />
  )
}

function AlertDialogAction({
  className,
  ...props
}: React.ComponentProps<typeof Button>) {
  return (
    <Button
      data-slot="alert-dialog-action"
      variant="ink"
      size="lg"
      className={cn(className)}
      {...props}
    />
  )
}

function AlertDialogCancel({
  className,
  variant = "outline",
  size = "lg",
  ...props
}: AlertDialogPrimitive.Close.Props &
  Pick<React.ComponentProps<typeof Button>, "variant" | "size">) {
  return (
    <AlertDialogPrimitive.Close
      data-slot="alert-dialog-cancel"
      className={cn(className)}
      render={<Button variant={variant} size={size} />}
      {...props}
    />
  )
}

export {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogTitle,
  AlertDialogTrigger,
}
