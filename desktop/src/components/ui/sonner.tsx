"use client"

import { Toaster as Sonner, type ToasterProps } from "sonner"

/*
 * shadcn sonner, unstyled and drawn as the design's toast: a dark pill above
 * the status bar with an accent dot. Sonner keeps the live region, the queue,
 * pause on hover and its own reduced-motion handling.
 */
const Toaster = ({ ...props }: ToasterProps) => {
  return (
    <Sonner
      theme="light"
      className="toaster group"
      position="bottom-center"
      offset={64}
      gap={8}
      style={{ "--width": "min(620px, calc(100vw - 48px))" } as React.CSSProperties}
      toastOptions={{
        unstyled: true,
        classNames: {
          toast:
            "left-0 right-0 mx-auto flex w-fit max-w-full items-center gap-2.5 rounded-[18px] bg-ink px-4 py-2 font-body fs-body text-bg shadow-toast before:size-1.5 before:shrink-0 before:self-[center] before:rounded-full before:bg-ac before:content-['']",
          content: "flex min-w-0 flex-col gap-0.5",
          title: "leading-snug",
          description: "fs-meta leading-snug text-bg/70 break-words",
          actionButton:
            "ml-1.5 shrink-0 rounded-[6px] px-1.5 py-0.5 font-semibold text-ac underline-offset-[3px] hover:underline focus-visible:outline-2 focus-visible:outline-bg",
          cancelButton: "ml-1 shrink-0 text-bg/70 hover:text-bg",
          icon: "hidden",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
