import { usePage } from '@inertiajs/react'
import { choice, t, type TextArgs } from '../tug/lang'

// useLang says a page's own words in the language the request came in, the
// one the server says its own in: the texts the app shares, which t() and
// choice() read. tug gen writes their TypeScript, and the rules a count's
// form follows, into ../tug/lang.ts; `tug lang vi` writes the texts a page
// says into lang/vi.json, to translate, as it does the Go's.
export function useLang() {
  const { texts } = usePage().props
  return {
    t: (text: string, args?: TextArgs) => t(texts, text, args),
    choice: (text: string, count: number, args?: TextArgs) => choice(texts, text, count, args),
  }
}
