import { page } from '@inertiajs/svelte'
import { onMount } from 'svelte'
import { choice, date, number, t, type DateLike, type TextArgs } from '../tug/lang'
import { foundHere, here } from './zone.svelte'

// useLang says a page's own words in the language the request came in, the
// one the server says its own in: the texts the app shares, which t() and
// choice() read. tug gen writes their TypeScript, and the rules a count's
// form follows, into ../tug/lang.ts; `tug lang vi` writes the texts a page
// says into lang/vi.json, to translate, as it does the Go's. n() and d()
// say a number and a time the way the language writes them.
export function useLang() {
  // Which way the language is written, for what lays itself out: a menu
  // opens from the side the language starts at. It's the page's, which
  // doesn't change under the browser's feet, as the document's own dir
  // doesn't.
  const dir: 'ltr' | 'rtl' = page.props.texts.dir === 'rtl' ? 'rtl' : 'ltr'
  // A time is written in the app's own zone, APP_TIMEZONE's, while the page
  // is rendered on the server, which can't know the reader's, and in the
  // reader's own once the page is in their browser: what the browser
  // hydrates has to be the markup the server sent.
  onMount(foundHere)
  return {
    t: (text: string, args?: TextArgs) => t(page.props.texts, text, args),
    choice: (text: string, count: number, args?: TextArgs) => choice(page.props.texts, text, count, args),
    n: (value: number, options?: Intl.NumberFormatOptions) => number(page.props.texts, value, options),
    d: (at: DateLike, options?: Intl.DateTimeFormatOptions) =>
      date(page.props.texts, at, { timeZone: here(), ...options }),
    dir,
  }
}
