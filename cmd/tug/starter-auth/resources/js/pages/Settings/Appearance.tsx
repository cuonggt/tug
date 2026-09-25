import { Head } from '@inertiajs/react'
import AppearanceTabs from '@/components/appearance-tabs'
import Heading from '@/components/heading'

// Appearance picks light, dark, or the system's, for this browser.
export default function Appearance() {
  return (
    <>
      <Head title="Appearance" />
      <section className="space-y-6">
        <Heading small title="Appearance" description="How the app looks in this browser." />
        <AppearanceTabs />
      </section>
    </>
  )
}
