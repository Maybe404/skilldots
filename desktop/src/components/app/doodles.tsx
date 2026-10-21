import { useApp } from "@/lib/app-state"
import { cn } from "@/lib/utils"

/**
 * The wobble filter of the two drawings in the doodle style. Always mounted.
 * Its region only covers those drawings (viewBoxes 90x90 and 150x90): WebKit
 * draws nothing for a filter whose region needs a buffer over about 4096x4096
 * device pixels, so never make it larger than what it filters. The wires do
 * not use it, see skills/wobble.ts.
 */
export function DoodleDefs() {
  return (
    <svg aria-hidden width="0" height="0" className="absolute">
      <defs>
        <filter id="skd-wob" filterUnits="userSpaceOnUse" x="-10" y="-10" width="170" height="110">
          <feTurbulence type="fractalNoise" baseFrequency="0.03" numOctaves="2" seed="11" />
          <feDisplacementMap in="SourceGraphic" scale="2.8" xChannelSelector="R" yChannelSelector="G" />
        </filter>
      </defs>
    </svg>
  )
}

/** Two small drawings from the design (moon and sprouts); doodle style only, decorative. */
export function Doodles() {
  const { style, view } = useApp()
  if (style !== "doodle") return null
  return (
    <>
      <svg
        aria-hidden
        // The editor has its own toolbar in that corner; the moon stays out of it.
        className={cn("pointer-events-none fixed top-[50px] right-[250px] z-[1] max-[960px]:hidden", (view === "editor" || view === "resolve") && "hidden")}
        width="56"
        height="56"
        viewBox="0 0 90 90"
        fill="none"
        stroke="var(--hue)"
        strokeWidth="1.6"
        strokeLinecap="round"
        filter="url(#skd-wob)"
      >
        <path d="M58 14c-20 4-30 26-18 44 8 12 24 16 36 10-16 0-28-10-30-26-1-12 4-22 12-28z" fill="var(--hue)" fillOpacity={0.14} />
        <path d="M26 40l1.5 3 3.3.5-2.4 2.3.6 3.3-3-1.6-3 1.6.6-3.3-2.4-2.3 3.3-.5z" fill="var(--ac)" fillOpacity={0.4} stroke="var(--ac)" />
        <path d="M44 50c1 2 4 2 5 0" stroke="var(--ink)" />
      </svg>
      <svg
        aria-hidden
        className="pointer-events-none fixed right-[30px] bottom-0 z-[5] opacity-90"
        width="110"
        height="48"
        viewBox="0 0 150 90"
        fill="none"
        stroke="var(--sage)"
        strokeWidth="1.5"
        strokeLinecap="round"
        filter="url(#skd-wob)"
      >
        <path d="M6 80c40-6 90-6 140 0" />
        <path d="M30 80c-2-20 6-36 16-44M46 36c-8 2-16 10-16 20M46 36c8 2 14 10 14 20" />
        <path d="M90 80c0-14 4-26 12-34M102 46c-6 0-12 6-12 14M102 46c6 0 12 6 12 14" stroke="var(--ac)" />
        <circle cx="46" cy="30" r="5" fill="var(--ac)" fillOpacity={0.3} stroke="var(--ac)" />
        <circle cx="102" cy="40" r="4" fill="var(--hue)" fillOpacity={0.3} stroke="var(--hue)" />
      </svg>
    </>
  )
}
