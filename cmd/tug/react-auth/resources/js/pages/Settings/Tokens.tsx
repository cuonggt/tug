import { Head, router, usePage } from '@inertiajs/react'
import { Check, Copy, KeyRound, LoaderCircle, Trash2 } from 'lucide-react'
import { type FormEvent, useEffect, useState } from 'react'
import Heading from '@/components/heading'
import InputError from '@/components/input-error'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// Tokens makes the user's API tokens, for a script, their phone's app or
// another service, which a request to /api sends in Authorization: Bearer
// in place of a login; shows a new one once; lists them; and revokes them.
// The handlers are in tokens.go.
export default function Tokens({ tokens, abilities }: PageProps<'Settings/Tokens'>) {
  const { flash, props } = usePage()
  // A new token comes once, in the flash, which the next visit empties, as
  // the bell's reload does when the token's notification rings it: kept
  // here, it's shown until the page is left.
  const [kept, setKept] = useState<string>()
  useEffect(() => {
    if (flash.token) setKept(flash.token)
  }, [flash.token])
  const newToken = flash.token ?? kept
  const [name, setName] = useState('')
  const [chosen, setChosen] = useState<string[]>([])
  const [expires, setExpires] = useState('30')
  const [making, setMaking] = useState(false)
  const [copied, setCopied] = useState(false)

  const make = (e: FormEvent) => {
    e.preventDefault()
    router.post(
      route('tokens.store'),
      { name, abilities: chosen, expires },
      {
        preserveScroll: true,
        onStart: () => setMaking(true),
        onSuccess: () => {
          setName('')
          setCopied(false)
        },
        onFinish: () => setMaking(false),
      },
    )
  }

  const copy = async (token: string) => {
    await navigator.clipboard.writeText(token)
    setCopied(true)
  }

  return (
    <>
      <Head title="API tokens" />
      <section className="space-y-6">
        <Heading
          small
          title="API tokens"
          description="For a script, your phone's app or another service, which sends one to the app's API in place of logging in."
        />
        {newToken && (
          <div className="space-y-3 rounded-lg border p-4">
            <p className="text-sm">Your new token. Copy it now: it won't be shown again.</p>
            <div className="flex items-start gap-2">
              <code data-testid="new-token" className="min-w-0 flex-1 rounded bg-muted px-2 py-1 font-mono text-sm break-all">
                {newToken}
              </code>
              <Button variant="outline" size="sm" onClick={() => copy(newToken)}>
                {copied ? <Check /> : <Copy />}
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </div>
          </div>
        )}
        {tokens.length > 0 && (
          <ul className="divide-y rounded-lg border">
            {tokens.map((token) => (
              <li key={token.id} className="flex items-center gap-3 p-3">
                <KeyRound className="size-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1 text-sm">
                  <p className="flex flex-wrap items-center gap-2 font-medium">
                    <span className="truncate">{token.name}</span>
                    {token.abilities.map((ability) => (
                      <Badge key={ability} variant="secondary">
                        {ability}
                      </Badge>
                    ))}
                  </p>
                  <p className="text-muted-foreground">
                    Made {day(token.createdAt)}
                    {token.lastUsedAt ? `, last used ${day(token.lastUsedAt)}` : ', not used yet'}
                    {token.expiresAt ? `, expires ${day(token.expiresAt)}` : ', never expires'}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Revoke ${token.name}`}
                  onClick={() => router.delete(route('tokens.destroy', { id: token.id }), { preserveScroll: true })}
                >
                  <Trash2 />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>
      <section className="space-y-6">
        <Heading small title="Make a token" description="Name it for what uses it, and give it no more than that needs." />
        <form onSubmit={make} className="space-y-6">
          <div className="grid gap-2">
            <Label htmlFor="token-name">Name</Label>
            <Input
              id="token-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My script"
              maxLength={100}
              required
            />
            <InputError message={props.errors.name} />
          </div>
          <fieldset className="grid gap-3">
            <legend className="mb-2 text-sm font-medium">What it may do</legend>
            {abilities.map((ability) => (
              <div key={ability} className="flex items-center gap-3">
                <Checkbox
                  id={`ability-${ability}`}
                  checked={chosen.includes(ability)}
                  onCheckedChange={(checked) =>
                    setChosen(checked === true ? [...chosen, ability] : chosen.filter((a) => a !== ability))
                  }
                />
                <Label htmlFor={`ability-${ability}`} className="font-mono font-normal">
                  {ability}
                </Label>
              </div>
            ))}
            <InputError message={props.errors.abilities} />
          </fieldset>
          <div className="grid gap-2">
            <Label htmlFor="token-expires">Expires</Label>
            <select
              id="token-expires"
              value={expires}
              onChange={(e) => setExpires(e.target.value)}
              className="h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 md:text-sm dark:bg-input/30"
            >
              <option value="30">In 30 days</option>
              <option value="365">In a year</option>
              <option value="0">Never</option>
            </select>
            <InputError message={props.errors.expires} />
          </div>
          <Button type="submit" disabled={making}>
            {making && <LoaderCircle className="animate-spin" />}
            Make a token
          </Button>
        </form>
      </section>
    </>
  )
}

// day is a date as the page shows it, the same on the server as in any
// browser, whatever its language or zone.
function day(at: string): string {
  return new Date(at).toLocaleDateString('en-US', { dateStyle: 'medium', timeZone: 'UTC' })
}
