// here is the reader's own time zone, once a page of the app is in their
// browser, and nothing while the server renders a page, where onMount
// doesn't run. It's a rune, in a .svelte.ts module as runes outside a
// component need, so a page that says a time says it again in the reader's
// zone the moment it's found.
let found = $state<string>()

export function here(): string | undefined {
  return found
}

export function foundHere() {
  found ??= Intl.DateTimeFormat().resolvedOptions().timeZone
}
