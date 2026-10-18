import { cn } from "cn"

import {
  Radio as RadioPrimitive,
  RadioGroup as RadioGroupPrimitive,
  RadioIndicator as RadioIndicatorPrimitive,
  type RadioGroupProps,
  type RadioProps,
} from "@/components/animate-ui/primitives/base/radio"

/*
 * Animate UI's Base UI radio group (components-base-radio): Base UI keeps the
 * radiogroup semantics and arrow keys; Animate UI pops the dot in on a spring
 * (fade only with reduced motion). Drawn as the design's 16px ring that fills
 * with the install hue.
 */
function RadioGroup({ className, ...props }: RadioGroupProps) {
  return <RadioGroupPrimitive className={cn("grid w-full gap-2", className)} {...props} />
}

function RadioGroupItem({ className, ...props }: RadioProps) {
  return (
    <RadioPrimitive
      className={cn(
        "peer relative flex aspect-square size-4 shrink-0 items-center justify-center rounded-full border-[1.5px] border-ln transition-colors after:absolute after:-inset-x-3 after:-inset-y-2 disabled:cursor-not-allowed disabled:opacity-50 data-checked:border-hue data-checked:bg-hue",
        className,
      )}
      {...props}
    >
      <RadioIndicatorPrimitive data-slot="radio-group-indicator" className="size-1.5 rounded-full bg-knob" />
    </RadioPrimitive>
  )
}

export { RadioGroup, RadioGroupItem }
