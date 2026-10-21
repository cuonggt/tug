import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react'
import { useAppearance, type Appearance } from '@/hooks/use-appearance'
import { useLang } from '@/lib/lang'
import { cn } from '@/lib/utils'
import { text } from '@/tug/lang'

const choices: { value: Appearance; icon: LucideIcon; label: string }[] = [
  { value: 'light', icon: Sun, label: text('Light') },
  { value: 'dark', icon: Moon, label: text('Dark') },
  { value: 'system', icon: Monitor, label: text('System') },
]

// AppearanceTabs choose light, dark, or the system's appearance.
export default function AppearanceTabs() {
  const { t } = useLang()

  const { appearance, setAppearance } = useAppearance()
  return (
    <div role="radiogroup" aria-label={t('Appearance')} className="inline-flex gap-1 rounded-lg bg-muted p-1">
      {choices.map(({ value, icon: Icon, label }) => (
        <button
          key={value}
          type="button"
          role="radio"
          aria-checked={appearance === value}
          onClick={() => setAppearance(value)}
          className={cn(
            'flex items-center gap-1.5 rounded-md px-3.5 py-1.5 text-sm transition-colors',
            appearance === value
              ? 'bg-background text-foreground shadow-xs'
              : 'text-muted-foreground hover:bg-background/60 hover:text-foreground',
          )}
        >
          <Icon className="size-4" />
          {t(label)}
        </button>
      ))}
    </div>
  )
}
