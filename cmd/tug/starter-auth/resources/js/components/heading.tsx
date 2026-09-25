// Heading is a section's title, and a line on what it's for.
export default function Heading({ title, description, small }: { title: string; description?: string; small?: boolean }) {
  return (
    <header className={small ? 'space-y-1' : 'mb-8 space-y-1'}>
      <h2 className={small ? 'text-base font-medium' : 'text-2xl font-semibold tracking-tight'}>{title}</h2>
      {description && <p className="text-sm text-muted-foreground">{description}</p>}
    </header>
  )
}
