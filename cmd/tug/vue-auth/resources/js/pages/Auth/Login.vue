<script setup lang="ts">
import { Form, Head, router, usePage } from '@inertiajs/vue3'
import { KeyRound, LoaderCircle } from '@lucide/vue'
import { onMounted, onUnmounted, ref } from 'vue'
import InputError from '@/components/InputError.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { vFocus } from '@/lib/focus'
import { autofillWorks, dismissed, getPasskey, type Json, optionsFrom, passkeysWork } from '@/lib/passkeys'
import { route } from '@/tug/routes'

// Login logs in with an email and password, or with a passkey, which the
// browser offers in the email field's autofill too. passkeyLogin in
// passkeys.go takes the passkey's.
defineOptions({
  layout: { title: 'Log in', description: 'Welcome back: your passkey, or your email and password, please.' },
})

const page = usePage()
const remember = ref(false)
const works = ref(false)
const failure = ref<string>()
let autofill: AbortController | undefined

const logIn = (credential: Json) => router.post(route('login.passkey'), { credential, remember: remember.value })

// The browser offers the site's passkeys as the email field is focused,
// for as long as the page is open.
onMounted(() => {
  works.value = passkeysWork()
  const abort = new AbortController()
  autofill = abort
  ;(async () => {
    if (!(await autofillWorks())) return
    const options = await optionsFrom(route('login.passkey.options'))
    if (options) logIn(await getPasskey(options, abort.signal))
  })().catch((err) => {
    if (!dismissed(err)) failure.value = "Your browser didn't use a passkey: try again, or use your password."
  })
})
onUnmounted(() => autofill?.abort())

const withPasskey = async () => {
  autofill?.abort() // a browser asks for one passkey at a time
  failure.value = undefined
  try {
    const options = await optionsFrom(route('login.passkey.options'))
    if (options) logIn(await getPasskey(options))
  } catch (err) {
    if (!dismissed(err)) failure.value = "Your browser didn't use a passkey: try again, or use your password."
  }
}
</script>

<template>
  <Head title="Log in" />
  <Form
    v-slot="{ errors, processing }"
    :action="route('login.store')"
    method="post"
    :reset-on-error="['password']"
    class="flex flex-col gap-6"
  >
    <div class="grid gap-2">
      <Label for="email">Email</Label>
      <Input
        id="email"
        v-focus
        name="email"
        type="email"
        autocomplete="username webauthn"
        placeholder="you@example.com"
        required
        :aria-invalid="!!errors.email"
      />
      <InputError :message="errors.email" />
    </div>
    <div class="grid gap-2">
      <div class="flex items-center">
        <Label for="password">Password</Label>
        <TextLink :href="route('password.request')" class="ml-auto text-sm">Forgotten it?</TextLink>
      </div>
      <PasswordInput id="password" name="password" autocomplete="current-password" required />
      <InputError :message="errors.password" />
    </div>
    <div class="flex items-center gap-3">
      <Checkbox
        id="remember"
        name="remember"
        :model-value="remember"
        @update:model-value="(checked) => (remember = checked === true)"
      />
      <Label for="remember" class="font-normal">Remember me for a month</Label>
    </div>
    <Button type="submit" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />Log in</Button>
  </Form>
  <div v-if="works" class="mt-4 grid gap-2">
    <Button type="button" variant="outline" class="w-full" @click="withPasskey"><KeyRound />Log in with a passkey</Button>
    <InputError :message="page.props.errors.passkey ?? failure" />
  </div>
  <p class="mt-6 text-center text-sm text-muted-foreground">No account yet? <TextLink :href="route('register')">Register</TextLink></p>
</template>
