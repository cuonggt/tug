import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

// cn joins class names, as shadcn-svelte's components do: conditions with
// clsx, and a later Tailwind class wins over an earlier one it clashes
// with, so cn('px-2', 'px-4') is 'px-4'.
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// initials are the first letters of a name's first and last words: "AL"
// for Ann Lee, which stand in for a user's photo while they have none.
export function initials(name: string) {
  const words = name.trim().split(/\s+/)
  const first = words[0]?.charAt(0) ?? ''
  const last = words.length > 1 ? words[words.length - 1].charAt(0) : ''
  return (first + last).toUpperCase()
}

// The types shadcn-svelte's components take their props with: a bits-ui
// component's props without the snippets it renders, and an element's with
// a ref to bind to it.
export type WithoutChild<T> = T extends { child?: any } ? Omit<T, 'child'> : T
export type WithoutChildren<T> = T extends { children?: any } ? Omit<T, 'children'> : T
export type WithoutChildrenOrChild<T> = WithoutChildren<WithoutChild<T>>
export type WithElementRef<T, U extends HTMLElement = HTMLElement> = T & { ref?: U | null }
