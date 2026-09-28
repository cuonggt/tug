import { Link, type InertiaLinkProps } from '@inertiajs/react'
import { cn } from '@/lib/utils'

// TextLink is a link within a sentence, underlined.
export default function TextLink({ className, ...props }: InertiaLinkProps) {
  return (
    <Link
      className={cn(
        'text-foreground underline decoration-neutral-300 underline-offset-4 transition-colors hover:decoration-current dark:decoration-neutral-500',
        className,
      )}
      {...props}
    />
  )
}
