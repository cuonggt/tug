// tug's site: the theme, the drawer of the guide's pages on a phone,
// search, copying code, and where the reader is on a page. Every page reads
// and works without it.
(() => {
  'use strict'

  const root = document.documentElement
  const base = root.dataset.base || '/'
  const $ = (selector, from = document) => from.querySelector(selector)
  const $$ = (selector, from = document) => [...from.querySelectorAll(selector)]

  // A screen reader hears what changed without its focus moving.
  const announce = (text) => {
    const region = $('[data-announce]')
    if (!region) return
    region.textContent = ''
    requestAnimationFrame(() => { region.textContent = text })
  }

  // A click on a modal dialog's backdrop lands on the dialog itself, outside
  // its box.
  const outside = (dialog, event) => {
    const box = dialog.getBoundingClientRect()
    return event.clientX < box.left || event.clientX > box.right ||
      event.clientY < box.top || event.clientY > box.bottom
  }

  // The theme: the system's, until the reader picks one, which is kept.

  const systemDark = matchMedia('(prefers-color-scheme: dark)')
  const isDark = () => (root.dataset.theme ? root.dataset.theme === 'dark' : systemDark.matches)
  const showTheme = () => {
    for (const button of $$('[data-theme-toggle]')) button.setAttribute('aria-pressed', String(isDark()))
    if (!root.dataset.theme) return
    // The browser's own bar follows the picked theme, not the system's.
    for (const meta of $$('meta[name="theme-color"]')) {
      meta.removeAttribute('media')
      meta.content = isDark() ? '#0b1120' : '#ffffff'
    }
  }
  for (const button of $$('[data-theme-toggle]')) {
    button.addEventListener('click', () => {
      const theme = isDark() ? 'light' : 'dark'
      root.dataset.theme = theme
      try { localStorage.setItem('theme', theme) } catch {}
      showTheme()
    })
  }
  systemDark.addEventListener('change', showTheme)
  showTheme()

  // Shortcuts are ⌘K on a Mac.
  if (/Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)) {
    for (const kbd of $$('[data-kbd]')) kbd.textContent = '⌘K'
  }

  // The drawer of the guide's pages, below the width the sidebar needs.

  const drawer = $('#nav-drawer')
  const openers = $$('[data-nav-open]')
  if (drawer) {
    for (const button of openers) {
      button.addEventListener('click', () => {
        drawer.showModal()
        button.setAttribute('aria-expanded', 'true')
        const current = $('[aria-current="page"]', drawer)
        if (current) current.scrollIntoView({ block: 'center' })
      })
    }
    drawer.addEventListener('close', () => {
      for (const button of openers) button.setAttribute('aria-expanded', 'false')
    })
    $('[data-nav-close]', drawer).addEventListener('click', () => drawer.close())
    drawer.addEventListener('click', (event) => {
      if (event.target === drawer && outside(drawer, event)) drawer.close()
    })
    matchMedia('(min-width: 1024px)').addEventListener('change', (event) => {
      if (event.matches && drawer.open) drawer.close()
    })
  }

  // Search, over search.json, fetched the first time it opens: each section
  // of the guide, by its heading, its page's title and its text.

  const search = $('#search')
  const input = $('#search-input')
  const results = $('#search-results')
  const note = $('[data-search-note]', search)
  const status = $('[data-search-status]', search)
  let sections = null
  let starts = []
  let loading = null
  let hits = []
  let active = -1
  let timer = 0

  const load = () => {
    loading ||= fetch(base + 'search.json')
      .then((response) => {
        if (!response.ok) throw new Error(`search.json: ${response.status}`)
        return response.json()
      })
      .then((index) => {
        sections = index.sections.map(([page, heading, id, text]) => {
          const [title, path] = index.pages[page]
          return {
            page,
            title,
            heading,
            text,
            url: base + path + (id ? '#' + id : ''),
            h: heading.toLowerCase(),
            t: title.toLowerCase(),
            x: text.toLowerCase(),
            intro: !id,
            history: path === 'docs/roadmap/',
          }
        })
        starts = index.pages.map(([title, path]) => ({ page: -1, title: 'Pages', heading: title, text: '', url: base + path }))
      })
    loading.catch(() => { loading = null })
    return loading
  }

  const wordStart = (text, at) => at === 0 || /[^\p{L}\p{N}]/u.test(text[at - 1])

  const occurrences = (text, term) => {
    let n = 0
    for (let at = text.indexOf(term); at >= 0 && n < 6; at = text.indexOf(term, at + term.length)) n++
    return n
  }

  // score ranks a section for the words searched for, each of which it must
  // have: in its heading most, then its page's title, then its text.
  const score = (s, terms, phrase) => {
    let total = 0
    for (const term of terms) {
      const inHeading = s.h.indexOf(term)
      const inTitle = s.t.indexOf(term)
      const inText = s.x.indexOf(term)
      if (inHeading < 0 && inTitle < 0 && inText < 0) return 0
      if (inHeading >= 0) total += wordStart(s.h, inHeading) ? 12 : 6
      if (inTitle >= 0) total += wordStart(s.t, inTitle) ? 4 : 2
      if (inText >= 0) total += occurrences(s.x, term) + (wordStart(s.x, inText) ? 1 : 0)
    }
    if (s.h === phrase) total += 20
    else if (terms.length > 1 && s.h.includes(phrase)) total += 12
    else if (terms.length > 1 && s.x.includes(phrase)) total += 5
    if (s.intro) total += 1
    // The roadmap is how tug got here: the guide first.
    return s.history ? total * 0.4 : total
  }

  const escapeRegExp = (text) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

  // mark writes text into el with the words searched for marked, as text,
  // never as HTML.
  const mark = (el, text, pattern) => {
    if (!pattern) {
      el.textContent = text
      return
    }
    let last = 0
    for (const match of text.matchAll(pattern)) {
      el.append(text.slice(last, match.index))
      const m = document.createElement('mark')
      m.textContent = match[0]
      el.append(m)
      last = match.index + match[0].length
    }
    el.append(text.slice(last))
  }

  // excerpt is the stretch of a section's text around the first word found.
  const excerpt = (text, terms) => {
    if (!text) return ''
    const lower = text.toLowerCase()
    let at = -1
    for (const term of terms) {
      const i = lower.indexOf(term)
      if (i >= 0 && (at < 0 || i < at)) at = i
    }
    if (at < 0) at = 0
    let start = Math.max(0, at - 60)
    if (start > 0) {
      const space = text.indexOf(' ', start)
      if (space >= 0 && space < at) start = space + 1
    }
    let end = Math.min(text.length, start + 180)
    if (end < text.length) {
      const space = text.lastIndexOf(' ', end)
      if (space > at) end = space
    }
    return (start > 0 ? '…' : '') + text.slice(start, end) + (end < text.length ? '…' : '')
  }

  const select = (i, scroll = true) => {
    if (hits[active]) hits[active].setAttribute('aria-selected', 'false')
    active = i
    const hit = hits[i]
    if (!hit) {
      input.removeAttribute('aria-activedescendant')
      return
    }
    hit.setAttribute('aria-selected', 'true')
    input.setAttribute('aria-activedescendant', hit.id)
    if (scroll) hit.scrollIntoView({ block: 'nearest' })
  }

  const render = (groups, terms) => {
    results.textContent = ''
    hits = []
    active = -1
    const pattern = terms.length ? new RegExp(terms.map(escapeRegExp).join('|'), 'giu') : null
    for (const [page, list] of groups) {
      const group = document.createElement('div')
      group.setAttribute('role', 'group')
      const label = document.createElement('div')
      label.className = 'search-group'
      label.id = `search-group-${page < 0 ? 'pages' : page}`
      label.textContent = list[0].title
      group.setAttribute('aria-labelledby', label.id)
      group.append(label)
      for (const s of list) {
        const hit = document.createElement('a')
        hit.className = 'search-hit'
        hit.href = s.url
        hit.id = `search-hit-${hits.length}`
        hit.tabIndex = -1
        hit.setAttribute('role', 'option')
        hit.setAttribute('aria-selected', 'false')
        const heading = document.createElement('span')
        heading.className = 'search-hit-title'
        mark(heading, s.heading, pattern)
        hit.append(heading)
        const snippet = terms.length ? excerpt(s.text, terms) : ''
        if (snippet) {
          const text = document.createElement('span')
          text.className = 'search-hit-text'
          mark(text, snippet, pattern)
          hit.append(text)
        }
        group.append(hit)
        hits.push(hit)
      }
      results.append(group)
    }
    input.setAttribute('aria-expanded', String(hits.length > 0))
    select(hits.length ? 0 : -1)
  }

  const showStart = () => {
    note.textContent = ''
    render(new Map([[-1, starts]]), [])
    status.textContent = ''
  }

  const failed = () => {
    results.textContent = ''
    hits = []
    note.textContent = "The search index didn't load. Try again in a moment, or browse the guide's parts."
  }

  const run = () => {
    const phrase = input.value.trim().toLowerCase().replace(/\s+/g, ' ')
    if (!sections) {
      load().then(run, failed)
      return
    }
    if (!phrase) {
      showStart()
      return
    }
    const terms = [...new Set(phrase.split(' '))]
    const found = []
    for (const s of sections) {
      const n = score(s, terms, phrase)
      if (n > 0) found.push([n, s])
    }
    found.sort((a, b) => b[0] - a[0])
    // Pages in the order of their best section, five sections a page.
    const groups = new Map()
    for (const [, s] of found) {
      let list = groups.get(s.page)
      if (!list) {
        if (groups.size === 8) continue
        list = []
        groups.set(s.page, list)
      }
      if (list.length < 5) list.push(s)
    }
    render(groups, terms)
    if (hits.length) {
      note.textContent = ''
      status.textContent = `${hits.length} result${hits.length === 1 ? '' : 's'}`
    } else {
      note.textContent = ''
      const strong = document.createElement('strong')
      strong.textContent = input.value.trim()
      note.append('Nothing in the guide for “', strong, '”. Try fewer words, or a package’s name, as queue or broadcast.')
      status.textContent = 'No results'
    }
  }

  const openSearch = () => {
    if (!search || search.open) return
    if (drawer && drawer.open) drawer.close()
    search.showModal()
    input.select()
    run()
  }

  const go = (url) => {
    search.close()
    location.href = url
  }

  if (search) {
    for (const button of $$('[data-search-open]')) button.addEventListener('click', openSearch)
    $('[data-search-close]', search).addEventListener('click', () => search.close())
    search.addEventListener('click', (event) => {
      if (event.target === search && outside(search, event)) search.close()
    })
    input.addEventListener('input', () => {
      clearTimeout(timer)
      timer = setTimeout(run, 50)
    })
    input.addEventListener('keydown', (event) => {
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault()
        if (!hits.length) return
        const step = event.key === 'ArrowDown' ? 1 : -1
        select((active + step + hits.length) % hits.length)
      } else if (event.key === 'Enter' && hits[active]) {
        event.preventDefault()
        go(hits[active].href)
      }
    })
    results.addEventListener('pointermove', (event) => {
      const hit = event.target.closest('.search-hit')
      if (hit && hits.indexOf(hit) !== active) select(hits.indexOf(hit), false)
    })
    results.addEventListener('click', (event) => {
      const hit = event.target.closest('.search-hit')
      // A link to a heading on this page scrolls to it, under the dialog.
      if (hit && !event.metaKey && !event.ctrlKey && !event.shiftKey) search.close()
    })
    document.addEventListener('keydown', (event) => {
      const typing = event.target instanceof HTMLElement &&
        (event.target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(event.target.tagName))
      if (event.key.toLowerCase() === 'k' && (event.metaKey || event.ctrlKey)) {
        event.preventDefault()
        if (search.open) search.close()
        else openSearch()
      } else if (event.key === '/' && !typing && !event.metaKey && !event.ctrlKey && !event.altKey) {
        event.preventDefault()
        openSearch()
      }
    })
  }

  // Copying code: a block's, or a button's own text.

  const copyText = async (text) => {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      const area = document.createElement('textarea')
      area.value = text
      area.setAttribute('readonly', '')
      area.style.position = 'fixed'
      area.style.opacity = '0'
      document.body.append(area)
      area.select()
      const ok = document.execCommand('copy')
      area.remove()
      return ok
    }
  }

  document.addEventListener('click', async (event) => {
    const button = event.target.closest('[data-copy]')
    if (!button) return
    const block = button.closest('.code')
    const text = button.dataset.copyText ?? (block ? $('pre', block).textContent : '')
    if (!(await copyText(text))) {
      announce("Couldn't copy: select the code and copy it")
      return
    }
    const label = $('[data-copy-label]', button)
    button.classList.add('is-copied')
    if (label) label.textContent = 'Copied'
    announce('Copied')
    clearTimeout(button.copied)
    button.copied = setTimeout(() => {
      button.classList.remove('is-copied')
      if (label) label.textContent = 'Copy'
    }, 1600)
  })

  // Where the reader is: the heading last scrolled past, in the page's
  // table of contents.

  const tocLinks = $$('.toc a')
  if (tocLinks.length) {
    const idOf = (a) => decodeURIComponent(a.hash.slice(1))
    const headings = [...new Set(tocLinks.map(idOf))].map((id) => document.getElementById(id)).filter(Boolean)
    const side = $('.toc-side')
    let current
    const update = () => {
      const top = (parseFloat(getComputedStyle(root).scrollPaddingTop) || 84) + 8
      let found = null
      for (const h of headings) {
        if (h.getBoundingClientRect().top > top) break
        found = h
      }
      // At the end of the page, the last heading is where the reader is,
      // however short its section.
      if (innerHeight + scrollY >= document.documentElement.scrollHeight - 2) found = headings[headings.length - 1]
      const id = found ? found.id : null
      if (id === current) return
      current = id
      for (const a of tocLinks) {
        if (idOf(a) === id) a.setAttribute('aria-current', 'true')
        else a.removeAttribute('aria-current')
      }
      const link = side && $('a[aria-current]', side)
      if (link && (link.offsetTop < side.scrollTop || link.offsetTop > side.scrollTop + side.clientHeight - 40)) {
        side.scrollTop = link.offsetTop - side.clientHeight / 3
      }
    }
    let queued = false
    addEventListener('scroll', () => {
      if (queued) return
      queued = true
      requestAnimationFrame(() => {
        queued = false
        update()
      })
    }, { passive: true })
    addEventListener('resize', update)
    update()

    // The table of contents in the page closes once it's taken the reader
    // somewhere.
    for (const a of $$('.toc-inline a')) {
      a.addEventListener('click', () => { a.closest('details').open = false })
    }
  }

  // Code and tables wider than the page scroll sideways; a keyboard can
  // reach them to scroll them.

  const scrollables = () => {
    for (const el of $$('.code pre, .table, .term-body')) {
      if (el.scrollWidth > el.clientWidth + 1) {
        el.tabIndex = 0
        el.setAttribute('role', 'region')
        el.setAttribute('aria-label', el.classList.contains('table') ? 'Table, scrolls sideways' : 'Code, scrolls sideways')
      } else if (el.hasAttribute('tabindex')) {
        el.removeAttribute('tabindex')
        el.removeAttribute('role')
        el.removeAttribute('aria-label')
      }
    }
  }
  let resizing = 0
  addEventListener('resize', () => {
    clearTimeout(resizing)
    resizing = setTimeout(scrollables, 150)
  })
  scrollables()
  if (document.fonts) document.fonts.ready.then(scrollables)

  if (document.readyState === 'complete') root.classList.add('loaded')
  else addEventListener('load', () => root.classList.add('loaded'))
})()
