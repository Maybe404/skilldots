import { Checkbox as BaseCheckbox } from "@base-ui/react/checkbox"
import { useReducedMotion } from "motion/react"
import { cn } from "cn"
import { CheckIcon, MinusIcon } from "lucide-react"

import {
  Checkbox as CheckboxPrimitive,
  CheckboxIndicator as CheckboxIndicatorPrimitive,
  type CheckboxProps as CheckboxPrimitiveProps,
} from "@/components/animate-ui/primitives/base/checkbox"

/*
 * Animate UI's Base UI checkbox (components-base-checkbox) drawn as the
 * design's 16px box (.skd-box): the tick is drawn stroke by stroke and the
 * box gives a little on press. With reduced motion the tick simply appears.
 */
function Checkbox({ className, ...props }: CheckboxPrimitiveProps) {
  const reduce = useReducedMotion()
  return (
    <CheckboxPrimitive
      data-slot="checkbox"
      whileHover={reduce ? undefined : { scale: 1.06 }}
      whileTap={reduce ? undefined : { scale: 0.92 }}
      className={cn("peer skd-box relative transition-colors after:absolute after:-inset-x-3 after:-inset-y-2 disabled:cursor-not-allowed disabled:opacity-45", className)}
      {...props}
    >
      {reduce ? (
        <BaseCheckbox.Indicator className="grid place-content-center [&>svg]:size-[11px] [&>svg]:stroke-[3.2]">
          <CheckIcon className="in-data-indeterminate:hidden" />
          <MinusIcon className="hidden in-data-indeterminate:block" />
        </BaseCheckbox.Indicator>
      ) : (
        <CheckboxIndicatorPrimitive className="size-[11px]" strokeWidth={3.2} />
      )}
    </CheckboxPrimitive>
  )
}

export { Checkbox }
