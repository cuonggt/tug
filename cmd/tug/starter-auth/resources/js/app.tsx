import { router } from '@inertiajs/react'
import { toast } from 'sonner'
import { followAppearance } from '@/hooks/use-appearance'
import { createApp } from './inertia'

// What a handler flashes with c.Flash("success", ...), or "error", shows
// as a toast. Listening from the start catches the flash that comes with
// the first page, as after following a link in the app's mail.
router.on('flash', (event) => {
  const { success, error } = event.detail.flash
  if (success) toast.success(success)
  if (error) toast.error(error)
})

followAppearance()

// The app, in the browser.
createApp()
