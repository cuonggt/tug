import { Link } from '@inertiajs/react'
import type { PageLink } from './tug/pages'

// A page of a list in numbered pages, as Go's tug.Paginate gives one: what
// a pager needs of it.
interface Page {
  current_page: number
  prev_page_url: string | null
  next_page_url: string | null
  links: PageLink[]
}

// Pager links a list's pages: the one before, the pages around this one,
// with "..." for those left out, and the one after. With go, its pages are
// buttons that ask for one by its number, as a page a QUERY rendered does,
// whose links, by GET, would go without what it was searched for.
export default function Pager({ page, go }: { page: Page; go?: (page: number) => void }) {
  const to = (label: string, number: number, url: string | null, current?: boolean) => {
    if (url === null) return <span key={label}>{label}</span>
    if (go) {
      return (
        <button key={label} type="button" onClick={() => go(number)} aria-current={current ? 'page' : undefined}>
          {label}
        </button>
      )
    }
    return (
      <Link key={label} href={url} aria-current={current ? 'page' : undefined}>
        {label}
      </Link>
    )
  }
  return (
    <nav aria-label="Pages" className="pager">
      {to('Previous', page.current_page - 1, page.prev_page_url)}
      {page.links.map((link, i) =>
        link.url === null ? <span key={`gap ${i}`}>{link.label}</span> : to(link.label, Number(link.label), link.url, link.active),
      )}
      {to('Next', page.current_page + 1, page.next_page_url)}
    </nav>
  )
}
