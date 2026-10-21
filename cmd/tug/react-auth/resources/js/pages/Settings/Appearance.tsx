import { Head } from '@inertiajs/react'
import AppearanceTabs from '@/components/appearance-tabs'
import Heading from '@/components/heading'
import { useLang } from '@/lib/lang'

// Appearance picks light, dark, or the system's, for this browser.
export default function Appearance() {
  const { t } = useLang()

  return (
    <>
      <Head title={t('Appearance')} />
      <section className="space-y-6">
        <Heading small title={t('Appearance')} description={t('How the app looks in this browser.')} />
        <AppearanceTabs />
      </section>
    </>
  )
}
