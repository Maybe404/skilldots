import { cn } from "cn"

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      aria-hidden
      className={cn("animate-pulse rounded-ctl bg-nav motion-reduce:animate-none", className)}
      {...props}
    />
  )
}

export { Skeleton }
