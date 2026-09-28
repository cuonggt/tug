import type { Directive } from 'vue'

// vFocus, as v-focus, puts the cursor in a field as it's mounted, as
// React's autoFocus does: on the field, or on a component that wraps one,
// as PasswordInput does. The autofocus attribute can't: a browser heeds
// only the first one a document loads with, not those on the pages
// Inertia swaps in after it, nor a field that appears later on a page.
export const vFocus: Directive<HTMLElement> = {
  mounted(el) {
    const field = el.matches('input, select, textarea') ? el : el.querySelector<HTMLElement>('input, select, textarea')
    field?.focus()
  },
}
