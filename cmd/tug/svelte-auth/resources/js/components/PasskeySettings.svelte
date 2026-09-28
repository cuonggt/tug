<script module lang="ts">
  // day is a date as the page shows it, the same on the server as in any
  // browser, whatever its language or zone.
  function day(at: string): string {
    return new Date(at).toLocaleDateString('en-US', { dateStyle: 'medium', timeZone: 'UTC' })
  }
</script>

<script lang="ts">
  import { page, router } from '@inertiajs/svelte'
  import KeyRound from '@lucide/svelte/icons/key-round'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import Trash2 from '@lucide/svelte/icons/trash-2'
  import { onMount } from 'svelte'
  import Heading from '@/components/Heading.svelte'
  import InputError from '@/components/InputError.svelte'
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
  let { passkeys }: PageProps<'Settings/Security'> = $props()
  let open = $state(false)
  let name = $state('')
  let adding = $state(false)
  let failure = $state<string>()
  // Known once the page is in the browser, so the server's HTML is the same.
  let works = $state<boolean>()
  onMount(() => {
    works = passkeysWork()
  })

  async function add(e: SubmitEvent) {
    e.preventDefault()
    adding = true
    failure = undefined
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
            open = false
            name = ''
          },
          onFinish: () => (adding = false),
        },
      )
    } catch (err) {
      adding = false
      if (err instanceof Refused) failure = err.message
      else if (!dismissed(err)) failure = "Your browser didn't make the passkey: try again."
    }
  }
</script>

<section class="space-y-6">
  <Heading
    small
    title="Passkeys"
    description="Log in with your phone, laptop or password manager, and its PIN, fingerprint or face: no password, and no code."
  />
  {#if passkeys.length > 0}
    <ul class="divide-y rounded-lg border">
      {#each passkeys as passkey (passkey.id)}
        <li class="flex items-center gap-3 p-3">
          <KeyRound class="size-4 shrink-0 text-muted-foreground" />
          <div class="min-w-0 flex-1 text-sm">
            <p class="flex items-center gap-2 font-medium">
              <span class="truncate">{passkey.name}</span>
              {#if passkey.synced}
                <Badge variant="secondary">Synced</Badge>
              {/if}
            </p>
            <p class="text-muted-foreground">
              Added {day(passkey.createdAt)}{passkey.lastUsedAt ? `, last used ${day(passkey.lastUsedAt)}` : ', not used yet'}
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label={`Remove ${passkey.name}`}
            onclick={() => router.delete(route('passkeys.destroy', { id: passkey.id }), { preserveScroll: true })}
          >
            <Trash2 />
          </Button>
        </li>
      {/each}
    </ul>
  {/if}
  <InputError message={page.props.errors.passkey} />
  {#if works}
    <Dialog bind:open>
      <DialogTrigger>
        {#snippet child({ props })}
          <Button {...props} variant="outline">
            <KeyRound />
            Add a passkey
          </Button>
        {/snippet}
      </DialogTrigger>
      <DialogContent>
        <DialogTitle>Add a passkey</DialogTitle>
        <DialogDescription>
          Name it for where it's kept, such as your phone, then your browser asks where to keep it.
        </DialogDescription>
        <form onsubmit={add} class="space-y-6">
          <div class="grid gap-2">
            <Label for="passkey-name">Name</Label>
            <Input
              id="passkey-name"
              bind:value={name}
              placeholder="My phone"
              maxlength={100}
              required
              autofocus
            />
            <InputError message={page.props.errors.name ?? failure} />
          </div>
          <DialogFooter class="gap-2">
            <DialogClose>
              {#snippet child({ props })}
                <Button {...props} type="button" variant="secondary">Cancel</Button>
              {/snippet}
            </DialogClose>
            <Button type="submit" disabled={adding}>
              {#if adding}
                <LoaderCircle class="animate-spin" />
              {/if}
              Add
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  {:else if works === false}
    <p class="text-sm text-muted-foreground">This browser doesn't make passkeys.</p>
  {/if}
</section>
