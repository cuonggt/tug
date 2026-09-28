<script setup lang="ts">
import { router, usePage } from '@inertiajs/vue3'
import { KeyRound, LoaderCircle, Trash2 } from '@lucide/vue'
import { onMounted, ref } from 'vue'
import Heading from '@/components/Heading.vue'
import InputError from '@/components/InputError.vue'
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
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// PasskeySettings lists the user's passkeys, which log in with no
// password, adds one, as the browser makes it, and removes them. The
// handlers are in passkeys.go.
defineProps<Pages['Settings/Security'] & SharedProps>()
const page = usePage()
const open = ref(false)
const name = ref('')
const adding = ref(false)
const failure = ref<string>()
// Known once the page is in the browser, so the server's HTML is the same.
const works = ref<boolean>()
onMounted(() => (works.value = passkeysWork()))

const add = async () => {
  adding.value = true
  failure.value = undefined
  try {
    const options = await optionsFrom(route('passkeys.options'))
    if (!options) return
    const credential = await createPasskey(options)
    router.post(
      route('passkeys.store'),
      { name: name.value, credential },
      {
        preserveScroll: true,
        onSuccess: () => {
          open.value = false
          name.value = ''
        },
        onFinish: () => (adding.value = false),
      },
    )
  } catch (err) {
    adding.value = false
    if (err instanceof Refused) failure.value = err.message
    else if (!dismissed(err)) failure.value = "Your browser didn't make the passkey: try again."
  }
}

// day is a date as the page shows it, the same on the server as in any
// browser, whatever its language or zone.
function day(at: string): string {
  return new Date(at).toLocaleDateString('en-US', { dateStyle: 'medium', timeZone: 'UTC' })
}
</script>

<template>
  <section class="space-y-6">
    <Heading
      small
      title="Passkeys"
      description="Log in with your phone, laptop or password manager, and its PIN, fingerprint or face: no password, and no code."
    />
    <ul v-if="passkeys.length > 0" class="divide-y rounded-lg border">
      <li v-for="passkey in passkeys" :key="passkey.id" class="flex items-center gap-3 p-3">
        <KeyRound class="size-4 shrink-0 text-muted-foreground" />
        <div class="min-w-0 flex-1 text-sm">
          <p class="flex items-center gap-2 font-medium">
            <span class="truncate">{{ passkey.name }}</span>
            <!-- A span, as shadcn's React Badge is: a div can't be in a p. -->
            <Badge v-if="passkey.synced" as="span" variant="secondary">Synced</Badge>
          </p>
          <p class="text-muted-foreground">Added {{ day(passkey.createdAt) }}{{ passkey.lastUsedAt ? `, last used ${day(passkey.lastUsedAt)}` : ', not used yet' }}</p>
        </div>
        <Button
          variant="ghost"
          size="icon"
          :aria-label="`Remove ${passkey.name}`"
          @click="router.delete(route('passkeys.destroy', { id: passkey.id }), { preserveScroll: true })"
        >
          <Trash2 />
        </Button>
      </li>
    </ul>
    <InputError :message="page.props.errors.passkey" />
    <Dialog v-if="works" v-model:open="open">
      <DialogTrigger as-child>
        <Button variant="outline"><KeyRound />Add a passkey</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogTitle>Add a passkey</DialogTitle>
        <DialogDescription>Name it for where it's kept, such as your phone, then your browser asks where to keep it.</DialogDescription>
        <form class="space-y-6" @submit.prevent="add">
          <div class="grid gap-2">
            <Label for="passkey-name">Name</Label>
            <!-- React's has autoFocus here; this has no v-focus. The dialog
                 puts the cursor in its first field itself, and as it closes
                 gives focus back to what had it as it opened: with v-focus,
                 that would be this field, gone by then, rather than the Add
                 a passkey button. -->
            <Input id="passkey-name" v-model="name" placeholder="My phone" maxlength="100" required />
            <InputError :message="page.props.errors.name ?? failure" />
          </div>
          <DialogFooter class="gap-2">
            <DialogClose as-child>
              <Button type="button" variant="secondary">Cancel</Button>
            </DialogClose>
            <Button type="submit" :disabled="adding"><LoaderCircle v-if="adding" class="animate-spin" />Add</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
    <p v-else-if="works === false" class="text-sm text-muted-foreground">This browser doesn't make passkeys.</p>
  </section>
</template>
