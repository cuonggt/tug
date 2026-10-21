import { usePage } from '@inertiajs/react'
import { useEffect, useState } from 'react'
import { choice, date, number, t, type DateLike, type TextArgs } from '../tug/lang'

// here is the reader's own time zone, once a page of the app is in their
// browser: one for the app, as every page is in the same browser, and
// never set while the server renders a page, where an effect doesn't run.
let here: string | undefined

// useLang says a page's own words in the language the request came in, the
// one the server says its own in: the texts the app shares, which t() and
// choice() read. tug gen writes their TypeScript, and the rules a count's
// form follows, into ../tug/lang.ts; `tug lang vi` writes the texts a page
// says into lang/vi.json, to translate, as it does the Go's. n() and d()
// say a number and a time the way the language writes them.
export function useLang() {
  const { texts } = usePage().props
  // A time is written in the app's own zone, APP_TIMEZONE's, while the page
  // is rendered on the server, which can't know the reader's, and in the
  // reader's own once the page is in their browser: what the browser
  // hydrates has to be the markup the server sent. A page visited after
  // that has the zone from the first, so its times don't change as it
  // opens.
  const [zone, setZone] = useState(here)
  useEffect(() => {
    here ??= Intl.DateTimeFormat().resolvedOptions().timeZone
    if (zone !== here) {
      setZone(here)
    }
  }, [zone])
  return {
    t: (text: string, args?: TextArgs) => t(texts, text, args),
    choice: (text: string, count: number, args?: TextArgs) => choice(texts, text, count, args),
    n: (value: number, options?: Intl.NumberFormatOptions) => number(texts, value, options),
    d: (at: DateLike, options?: Intl.DateTimeFormatOptions) => date(texts, at, { timeZone: zone, ...options }),
  }
}
