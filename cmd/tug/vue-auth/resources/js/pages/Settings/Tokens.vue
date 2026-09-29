<script setup lang="ts">
import { Head, router, usePage } from '@inertiajs/vue3'
import { Check, Copy, KeyRound, LoaderCircle, Trash2 } from '@lucide/vue'
import { ref } from 'vue'
import Heading from '@/components/Heading.vue'
import InputError from '@/components/InputError.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// Tokens makes the user's API tokens, for a script, their phone's app or
// another service, which a request to /api sends in Authorization: Bearer
// in place of a login; shows a new one once; lists them; and revokes them.
// The handlers are in tokens.go.
defineProps<Pages['Settings/Tokens'] & SharedProps>()
const page = usePage()
const name = ref('')
const chosen = ref<string[]>([])
const expires = ref('30')
const making = ref(false)
const copied = ref(false)

const make = () => {
  router.post(
    route('tokens.store'),
    { name: name.value, abilities: chosen.value, expires: expires.value },
    {
      preserveScroll: true,
      onStart: () => (making.value = true),
      onSuccess: () => {
        name.value = ''
        copied.value = false
      },
      onFinish: () => (making.value = false),
    },
  )
}

const choose = (ability: string, checked: boolean | 'indeterminate') => {
  chosen.value = checked === true ? [...chosen.value, ability] : chosen.value.filter((a) => a !== ability)
}

const copy = async (token: string) => {
  await navigator.clipboard.writeText(token)
  copied.value = true
}

// day is a date as the page shows it, the same on the server as in any
// browser, whatever its language or zone.
function day(at: string): string {
  return new Date(at).toLocaleDateString('en-US', { dateStyle: 'medium', timeZone: 'UTC' })
}
</script>

<template>
  <Head title="API tokens" />
  <section class="space-y-6">
    <Heading
      small
      title="API tokens"
      description="For a script, your phone's app or another service, which sends one to the app's API in place of logging in."
    />
    <div v-if="page.flash.token" class="space-y-3 rounded-lg border p-4">
      <p class="text-sm">Your new token. Copy it now: it won't be shown again.</p>
      <div class="flex items-start gap-2">
        <code data-testid="new-token" class="min-w-0 flex-1 rounded bg-muted px-2 py-1 font-mono text-sm break-all">{{ page.flash.token }}</code>
        <Button variant="outline" size="sm" @click="copy(page.flash.token!)">
          <Check v-if="copied" />
          <Copy v-else />
          {{ copied ? 'Copied' : 'Copy' }}
        </Button>
      </div>
    </div>
    <ul v-if="tokens.length > 0" class="divide-y rounded-lg border">
      <li v-for="token in tokens" :key="token.id" class="flex items-center gap-3 p-3">
        <KeyRound class="size-4 shrink-0 text-muted-foreground" />
        <div class="min-w-0 flex-1 text-sm">
          <p class="flex flex-wrap items-center gap-2 font-medium">
            <span class="truncate">{{ token.name }}</span>
            <!-- A span, as shadcn's React Badge is: a div can't be in a p. -->
            <Badge v-for="ability in token.abilities" :key="ability" as="span" variant="secondary">{{ ability }}</Badge>
          </p>
          <p class="text-muted-foreground">
            Made {{ day(token.createdAt) }}{{ token.lastUsedAt ? `, last used ${day(token.lastUsedAt)}` : ', not used yet'
            }}{{ token.expiresAt ? `, expires ${day(token.expiresAt)}` : ', never expires' }}
          </p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          :aria-label="`Revoke ${token.name}`"
          @click="router.delete(route('tokens.destroy', { id: token.id }), { preserveScroll: true })"
        >
          <Trash2 />
        </Button>
      </li>
    </ul>
  </section>
  <section class="space-y-6">
    <Heading small title="Make a token" description="Name it for what uses it, and give it no more than that needs." />
    <form class="space-y-6" @submit.prevent="make">
      <div class="grid gap-2">
        <Label for="token-name">Name</Label>
        <Input id="token-name" v-model="name" placeholder="My script" maxlength="100" required />
        <InputError :message="page.props.errors.name" />
      </div>
      <fieldset class="grid gap-3">
        <legend class="mb-2 text-sm font-medium">What it may do</legend>
        <div v-for="ability in abilities" :key="ability" class="flex items-center gap-3">
          <Checkbox
            :id="`ability-${ability}`"
            :model-value="chosen.includes(ability)"
            @update:model-value="(checked) => choose(ability, checked)"
          />
          <Label :for="`ability-${ability}`" class="font-mono font-normal">{{ ability }}</Label>
        </div>
        <InputError :message="page.props.errors.abilities" />
      </fieldset>
      <div class="grid gap-2">
        <Label for="token-expires">Expires</Label>
        <select
          id="token-expires"
          v-model="expires"
          class="h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 md:text-sm dark:bg-input/30"
        >
          <option value="30">In 30 days</option>
          <option value="365">In a year</option>
          <option value="0">Never</option>
        </select>
        <InputError :message="page.props.errors.expires" />
      </div>
      <Button type="submit" :disabled="making">
        <LoaderCircle v-if="making" class="animate-spin" />
        Make a token
      </Button>
    </form>
  </section>
</template>
