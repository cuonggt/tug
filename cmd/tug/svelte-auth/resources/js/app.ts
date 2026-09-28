import { router } from '@inertiajs/svelte'
import { mount } from 'svelte'
import { toast } from 'svelte-sonner'
import { Toaster } from '@/components/ui/sonner'
import { followAppearance } from '@/lib/appearance.svelte'
import { createApp } from './inertia'

// What a handler flashes with c.Flash("success", ...), or "error", shows
// as a toast. Listening from the start catches the flash that comes with
// the first page, as after following a link in the app's mail.
router.on('flash', (event) => {
  const { success, error } = event.detail.flash
  if (success) toast.success(success)
  if (error) toast.error(error)
})

// The toasts are mounted once, on an element of their own beside the app's,
// rather than in a layout: a flash often comes with a change of page and
// of layout, as "You've logged out." does, on Home, which has none.
mount(Toaster, { target: document.body.appendChild(document.createElement('div')) })

// The app, in the browser. The components show the appearance chosen once
// it has taken over the page the server rendered, which can't know it.
await createApp()

followAppearance()
