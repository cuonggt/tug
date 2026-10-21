<script setup lang="ts">
import { createForm, Head, router, usePage } from '@inertiajs/vue3'
import { KeyRound, LoaderCircle } from '@lucide/vue'
import { onMounted, onUnmounted, ref } from 'vue'
import InputError from '@/components/InputError.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
  import { useLang } from '@/lib/lang'
import { vFocus } from '@/lib/focus'
import { autofillWorks, dismissed, getPasskey, type Json, optionsFrom, passkeysWork } from '@/lib/passkeys'
  import { text } from '@/tug/lang'
import { form, route, type Inputs } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

// Login logs in with an email and password, or with a passkey, which the
// browser offers in the email field's autofill too. passkeyLogin in
// passkeys.go takes the passkey's.
defineOptions({
  layout: { title: text('Log in'), description: text('Welcome back: your passkey, or your email and password, please.') },
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
    if (!dismissed(err)) failure.value = t("Your browser didn't use a passkey: try again, or use your password.")
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
    if (!dismissed(err)) failure.value = t("Your browser didn't use a passkey: try again, or use your password.")
  }
}

// The form, typed by its route's input, as tug gen writes it from the Go:
// its errors are the fields it sends.
const LoginForm = createForm<Inputs['login.store']>()
</script>

<template>
  <Head :title="t('Log in')" />
  <LoginForm
    v-slot="{ errors, processing }"
    :action="form('login.store')"
    :reset-on-error="['password']"
    class="flex flex-col gap-6"
  >
    <div class="grid gap-2">
      <Label for="email">{{ t('Email') }}</Label>
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
        <Label for="password">{{ t('Password') }}</Label>
        <TextLink :href="route('password.request')" class="ml-auto text-sm">{{ t('Forgotten it?') }}</TextLink>
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
      <Label for="remember" class="font-normal">{{ t('Remember me for a month') }}</Label>
    </div>
    <Button type="submit" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />{{ t('Log in') }}</Button>
  </LoginForm>
  <div v-if="works" class="mt-4 grid gap-2">
    <Button type="button" variant="outline" class="w-full" @click="withPasskey"><KeyRound />{{ t('Log in with a passkey') }}</Button>
    <InputError :message="page.props.errors.passkey ?? failure" />
  </div>
  <p class="mt-6 text-center text-sm text-muted-foreground">{{ t('No account yet?') }} <TextLink :href="route('register')">{{ t('Register') }}</TextLink></p>
</template>
