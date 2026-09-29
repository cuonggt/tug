import { Link } from '@inertiajs/react'
import type { PageLink } from './tug/pages'

// A page of a list in numbered pages, as Go's tug.Paginate gives one: what
// a pager needs of it.
interface Page {
  prev_page_url: string | null
  next_page_url: string | null
  links: PageLink[]
}

// Pager links a list's pages: the one before, the pages around this one,
// with "..." for those left out, and the one after.
export default function Pager({ page }: { page: Page }) {
  return (
    <nav aria-label="Pages" className="pager">
      {page.prev_page_url ? <Link href={page.prev_page_url}>Previous</Link> : <span>Previous</span>}
      {page.links.map((link, i) =>
        link.url === null ? (
          <span key={i}>{link.label}</span>
        ) : (
          <Link key={i} href={link.url} aria-current={link.active ? 'page' : undefined}>
            {link.label}
          </Link>
        ),
      )}
      {page.next_page_url ? <Link href={page.next_page_url}>Next</Link> : <span>Next</span>}
    </nav>
  )
}
