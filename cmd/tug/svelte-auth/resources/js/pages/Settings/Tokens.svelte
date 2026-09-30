<script module lang="ts">
  // day is a date as the page shows it, the same on the server as in any
  // browser, whatever its language or zone.
  function day(at: string): string {
    return new Date(at).toLocaleDateString('en-US', { dateStyle: 'medium', timeZone: 'UTC' })
  }
</script>

<script lang="ts">
  import { page, router } from '@inertiajs/svelte'
  import Check from '@lucide/svelte/icons/check'
  import Copy from '@lucide/svelte/icons/copy'
  import KeyRound from '@lucide/svelte/icons/key-round'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import Trash2 from '@lucide/svelte/icons/trash-2'
  import Head from '@/Head.svelte'
  import Heading from '@/components/Heading.svelte'
  import InputError from '@/components/InputError.svelte'
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
  let { tokens, abilities }: PageProps<'Settings/Tokens'> = $props()
  // A new token comes once, in the flash, which the next visit empties, as
  // the bell's reload does when the token's notification rings it: kept
  // here, it's shown until the page is left.
  let kept = $state<string>()
  $effect(() => {
    if (page.flash.token) kept = page.flash.token
  })
  let newToken = $derived(page.flash.token ?? kept)
  let name = $state('')
  let chosen = $state<string[]>([])
  let expires = $state('30')
  let making = $state(false)
  let copied = $state(false)

  function make(e: SubmitEvent) {
    e.preventDefault()
    router.post(
      route('tokens.store'),
      { name, abilities: chosen, expires },
      {
        preserveScroll: true,
        onStart: () => (making = true),
        onSuccess: () => {
          name = ''
          copied = false
        },
        onFinish: () => (making = false),
      },
    )
  }

  function choose(ability: string, checked: boolean) {
    chosen = checked ? [...chosen, ability] : chosen.filter((a) => a !== ability)
  }

  async function copy(token: string) {
    await navigator.clipboard.writeText(token)
    copied = true
  }
</script>

<Head title="API tokens" />
<section class="space-y-6">
  <Heading
    small
    title="API tokens"
    description="For a script, your phone's app or another service, which sends one to the app's API in place of logging in."
  />
  {#if newToken}
    <div class="space-y-3 rounded-lg border p-4">
      <p class="text-sm">Your new token. Copy it now: it won't be shown again.</p>
      <div class="flex items-start gap-2">
        <code data-testid="new-token" class="min-w-0 flex-1 rounded bg-muted px-2 py-1 font-mono text-sm break-all">{newToken}</code>
        <Button variant="outline" size="sm" onclick={() => copy(newToken!)}>
          {#if copied}
            <Check />
            Copied
          {:else}
            <Copy />
            Copy
          {/if}
        </Button>
      </div>
    </div>
  {/if}
  {#if tokens.length > 0}
    <ul class="divide-y rounded-lg border">
      {#each tokens as token (token.id)}
        <li class="flex items-center gap-3 p-3">
          <KeyRound class="size-4 shrink-0 text-muted-foreground" />
          <div class="min-w-0 flex-1 text-sm">
            <p class="flex flex-wrap items-center gap-2 font-medium">
              <span class="truncate">{token.name}</span>
              {#each token.abilities as ability (ability)}
                <Badge variant="secondary">{ability}</Badge>
              {/each}
            </p>
            <p class="text-muted-foreground">
              Made {day(token.createdAt)}{token.lastUsedAt ? `, last used ${day(token.lastUsedAt)}` : ', not used yet'}{token.expiresAt
                ? `, expires ${day(token.expiresAt)}`
                : ', never expires'}
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label={`Revoke ${token.name}`}
            onclick={() => router.delete(route('tokens.destroy', { id: token.id }), { preserveScroll: true })}
          >
            <Trash2 />
          </Button>
        </li>
      {/each}
    </ul>
  {/if}
</section>
<section class="space-y-6">
  <Heading small title="Make a token" description="Name it for what uses it, and give it no more than that needs." />
  <form onsubmit={make} class="space-y-6">
    <div class="grid gap-2">
      <Label for="token-name">Name</Label>
      <Input id="token-name" bind:value={name} placeholder="My script" maxlength={100} required />
      <InputError message={page.props.errors.name} />
    </div>
    <fieldset class="grid gap-3">
      <legend class="mb-2 text-sm font-medium">What it may do</legend>
      {#each abilities as ability (ability)}
        <div class="flex items-center gap-3">
          <Checkbox
            id={`ability-${ability}`}
            checked={chosen.includes(ability)}
            onCheckedChange={(checked) => choose(ability, checked === true)}
          />
          <Label for={`ability-${ability}`} class="font-mono font-normal">{ability}</Label>
        </div>
      {/each}
      <InputError message={page.props.errors.abilities} />
    </fieldset>
    <div class="grid gap-2">
      <Label for="token-expires">Expires</Label>
      <select
        id="token-expires"
        bind:value={expires}
        class="h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 md:text-sm dark:bg-input/30"
      >
        <option value="30">In 30 days</option>
        <option value="365">In a year</option>
        <option value="0">Never</option>
      </select>
      <InputError message={page.props.errors.expires} />
    </div>
    <Button type="submit" disabled={making}>
      {#if making}
        <LoaderCircle class="animate-spin" />
      {/if}
      Make a token
    </Button>
  </form>
</section>
