import { router } from '@inertiajs/vue3'
import { createApp as createVueApp, h } from 'vue'
import { toast } from 'vue-sonner'
import 'vue-sonner/style.css'
import { Toaster } from '@/components/ui/sonner'
import { followAppearance } from '@/composables/useAppearance'
import { createApp } from './inertia'

// What a handler flashes with c.Flash("success", ...), or "error", shows
// as a toast. Listening from the start catches the flash that comes with
// the first page, as after following a link in the app's mail.
router.on('flash', (event) => {
  const { success, error } = event.detail.flash
  if (success) toast.success(success)
  if (error) toast.error(error)
})

// The toasts are an app of their own, beside the page's: Inertia's Vue app
// renders only the page, and in a layout they'd go as the page changes
// layout, as logging out does, to the landing page, which has none. It's
// mounted first, to be there for the first page's flash, and vue-sonner's
// styles come with it, which React's sonner adds by itself.
const toaster = document.createElement('div')
document.body.append(toaster)
createVueApp({ render: () => h(Toaster) }).mount(toaster)

followAppearance()

// The app, in the browser.
createApp()
