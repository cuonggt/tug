import { router, usePage } from '@inertiajs/react'
import { KeyRound, LoaderCircle, Trash2 } from 'lucide-react'
import { type FormEvent, useEffect, useState } from 'react'
import Heading from '@/components/heading'
import InputError from '@/components/input-error'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { createPasskey, dismissed, optionsFrom, passkeysWork, Refused } from '@/lib/passkeys'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// PasskeySettings lists the user's passkeys, which log in with no
// password, adds one, as the browser makes it, and removes them. The
// handlers are in passkeys.go.
export default function PasskeySettings({ passkeys }: PageProps<'Settings/Security'>) {
  const { errors } = usePage().props
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [adding, setAdding] = useState(false)
  const [failure, setFailure] = useState<string>()
  // Known once the page is in the browser, so the server's HTML is the same.
  const [works, setWorks] = useState<boolean>()
  useEffect(() => setWorks(passkeysWork()), [])

  const add = async (e: FormEvent) => {
    e.preventDefault()
    setAdding(true)
    setFailure(undefined)
    try {
      const options = await optionsFrom(route('passkeys.options'))
      if (!options) return
      const credential = await createPasskey(options)
      router.post(
        route('passkeys.store'),
        { name, credential },
        {
          preserveScroll: true,
          onSuccess: () => {
            setOpen(false)
            setName('')
          },
          onFinish: () => setAdding(false),
        },
      )
    } catch (err) {
      setAdding(false)
      if (err instanceof Refused) setFailure(err.message)
      else if (!dismissed(err)) setFailure("Your browser didn't make the passkey: try again.")
    }
  }

  return (
    <section className="space-y-6">
      <Heading
        small
        title="Passkeys"
        description="Log in with your phone, laptop or password manager, and its PIN, fingerprint or face: no password, and no code."
      />
      {passkeys.length > 0 && (
        <ul className="divide-y rounded-lg border">
          {passkeys.map((passkey) => (
            <li key={passkey.id} className="flex items-center gap-3 p-3">
              <KeyRound className="size-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1 text-sm">
                <p className="flex items-center gap-2 font-medium">
                  <span className="truncate">{passkey.name}</span>
                  {passkey.synced && <Badge variant="secondary">Synced</Badge>}
                </p>
                <p className="text-muted-foreground">
                  Added {day(passkey.createdAt)}
                  {passkey.lastUsedAt ? `, last used ${day(passkey.lastUsedAt)}` : ', not used yet'}
                </p>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`Remove ${passkey.name}`}
                onClick={() => router.delete(route('passkeys.destroy', { id: passkey.id }), { preserveScroll: true })}
              >
                <Trash2 />
              </Button>
            </li>
          ))}
        </ul>
      )}
      <InputError message={errors.passkey} />
      {works ? (
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button variant="outline">
              <KeyRound />
              Add a passkey
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogTitle>Add a passkey</DialogTitle>
            <DialogDescription>
              Name it for where it's kept, such as your phone, then your browser asks where to keep it.
            </DialogDescription>
            <form onSubmit={add} className="space-y-6">
              <div className="grid gap-2">
                <Label htmlFor="passkey-name">Name</Label>
                <Input
                  id="passkey-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="My phone"
                  maxLength={100}
                  required
                  autoFocus
                />
                <InputError message={errors.name ?? failure} />
              </div>
              <DialogFooter className="gap-2">
                <DialogClose asChild>
                  <Button type="button" variant="secondary">
                    Cancel
                  </Button>
                </DialogClose>
                <Button type="submit" disabled={adding}>
                  {adding && <LoaderCircle className="animate-spin" />}
                  Add
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      ) : (
        works === false && <p className="text-sm text-muted-foreground">This browser doesn't make passkeys.</p>
      )}
    </section>
  )
}

// day is a date as the page shows it, the same on the server as in any
// browser, whatever its language or zone.
function day(at: string): string {
  return new Date(at).toLocaleDateString('en-US', { dateStyle: 'medium', timeZone: 'UTC' })
}
