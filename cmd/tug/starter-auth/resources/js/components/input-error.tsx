import { cn } from '@/lib/utils'

// InputError is what the server said is wrong with a field, under it.
export default function InputError({ message, className }: { message?: string; className?: string }) {
  if (!message) return null
  return <p className={cn('text-sm text-destructive', className)}>{message}</p>
}
