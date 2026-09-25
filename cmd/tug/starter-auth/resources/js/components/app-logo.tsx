import { usePage } from '@inertiajs/react'
import { cn } from '@/lib/utils'

// AppLogoIcon is the app's mark: the first letter of its name, until it
// has a logo of its own to put here.
export function AppLogoIcon({ className }: { className?: string }) {
  const { appName } = usePage().props
  return (
    <span
      aria-hidden
      className={cn(
        'flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground',
        className,
      )}
    >
      {appName.charAt(0).toUpperCase()}
    </span>
  )
}

// AppLogo is the mark and the name, as the header has them.
export default function AppLogo() {
  const { appName } = usePage().props
  return (
    <span className="flex items-center gap-2">
      <AppLogoIcon />
      <span className="font-semibold tracking-tight">{appName}</span>
    </span>
  )
}
