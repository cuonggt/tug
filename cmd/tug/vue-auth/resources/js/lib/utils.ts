import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

// cn joins class names, as shadcn's components do: conditions with clsx,
// and a later Tailwind class wins over an earlier one it clashes with, so
// cn('px-2', 'px-4') is 'px-4'.
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
