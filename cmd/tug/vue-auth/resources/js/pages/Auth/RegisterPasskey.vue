<script setup lang="ts">
import { Head, router, usePage } from '@inertiajs/vue3'
import { KeyRound, LoaderCircle } from '@lucide/vue'
import { onMounted, ref } from 'vue'
import InputError from '@/components/InputError.vue'
import { Button } from '@/components/ui/button'
import { createPasskey, dismissed, optionsFrom, passkeysWork, Refused } from '@/lib/passkeys'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// RegisterPasskey is the page the link mailed to register with a passkey
// opens: the browser makes the passkey, and registerPasskey in passkeys.go
// makes the account with it, its email verified by the link, with no
// password.
defineOptions({
  layout: {
    title: 'Make your passkey',
    description: 'It logs you in with your phone, laptop or password manager, and its PIN, fingerprint or face.',
  },
})
const props = defineProps<Pages['Auth/RegisterPasskey'] & SharedProps>()
const page = usePage()
const making = ref(false)
const failure = ref<string>()
// Known once the page is in the browser, so the server's HTML is the same.
const works = ref<boolean>()
onMounted(() => (works.value = passkeysWork()))

const make = async () => {
  making.value = true
  failure.value = undefined
  try {
    const query = new URLSearchParams({ email: props.email, name: props.name })
    const options = await optionsFrom(`${route('register.passkey.options', { token: props.token })}?${query}`)
    if (!options) return
    const credential = await createPasskey(options)
    router.post(
      route('register.passkey.store', { token: props.token }),
      { email: props.email, name: props.name, credential },
      { onFinish: () => (making.value = false) },
    )
  } catch (err) {
    making.value = false
    if (err instanceof Refused) failure.value = err.message
    else if (!dismissed(err)) failure.value = "Your browser didn't make the passkey: try again."
  }
}
</script>

<template>
  <Head title="Make your passkey" />
  <div class="flex flex-col gap-6">
    <p class="text-sm">
      Your account is <strong>{{ name }}</strong>, at <strong>{{ email }}</strong>, with no password: your browser asks where to keep the passkey.
    </p>
    <div class="grid gap-2">
      <Button type="button" class="w-full" :disabled="making || !works" @click="make">
        <LoaderCircle v-if="making" class="animate-spin" /><KeyRound v-else />Make your passkey
      </Button>
      <InputError :message="page.props.errors.passkey ?? failure" />
      <p v-if="works === false" class="text-sm text-muted-foreground">This browser doesn't make passkeys: open the link in one that does.</p>
    </div>
  </div>
</template>
