<script setup lang="ts">
import type { FormComponentRef } from '@inertiajs/core'
import { createForm, Head } from '@inertiajs/vue3'
import { KeyRound, LoaderCircle, LockKeyhole } from '@lucide/vue'
import { onBeforeUnmount, onMounted, ref, useTemplateRef } from 'vue'
import InputError from '@/components/InputError.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
  import { useLang } from '@/lib/lang'
import { vFocus } from '@/lib/focus'
import { passkeysWork } from '@/lib/passkeys'
  import { text } from '@/tug/lang'
import { form, route, type Inputs } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

// Register checks each field with the server as it's left, as BindValid in
// auth.go answers: a taken email shows before the form is sent. Where the
// browser makes passkeys, the account can have one in place of a password:
// the form sends the name and the email alone, and registerWithPasskey in
// passkeys.go mails a link, whose page makes the passkey and the account.
defineOptions({
  layout: { title: text('Make an account'), description: text('Your name, your email, and a password of 8 characters or more, or a passkey.') },
})

// Known once the page is in the browser, so the server's HTML is the same.
const works = ref(false)
const withPasskey = ref(false)
onMounted(() => (works.value = passkeysWork()))

// A field's check can be waiting out its 300ms as the form is sent, and
// Inertia's Vue Form throws when it runs: after registering has logged the
// browser in, guestsOnly redirects it, which isn't Precognition's answer,
// and once the next page has taken this one's place, it reads the form
// that's gone (@inertiajs/vue3 3.7.1; React's Form sets its timeout afresh
// each time it renders, as when it's sent, and checks for the form). So
// it's dropped as the form is sent, and as the page goes: setting the
// validator's timeout makes it afresh, with nothing waiting.
const registration = useTemplateRef<FormComponentRef>('registration')
const dropCheck = () => registration.value?.validator().setTimeout(300)
onBeforeUnmount(dropCheck)

// The form, typed by its route's input, as tug gen writes it from the Go:
// its errors are the fields it sends.
const RegisterForm = createForm<Inputs['register.store']>()
</script>

<template>
  <Head :title="t('Register')" />
  <RegisterForm
    ref="registration"
    v-slot="{ errors, processing, validate, invalid }"
    :action="withPasskey ? form('register.passkey') : form('register.store')"
    :reset-on-error="['password', 'password_confirmation']"
    :validation-timeout="300"
    class="flex flex-col gap-6"
    @start="dropCheck"
  >
    <div class="grid gap-2">
      <Label for="name">{{ t('Name') }}</Label>
      <Input
        id="name"
        v-focus
        name="name"
        autocomplete="name"
        required
        :aria-invalid="invalid('name')"
        @blur="validate('name')"
      />
      <InputError :message="errors.name" />
    </div>
    <div class="grid gap-2">
      <Label for="email">{{ t('Email') }}</Label>
      <Input
        id="email"
        name="email"
        type="email"
        autocomplete="email"
        placeholder="you@example.com"
        required
        :aria-invalid="invalid('email')"
        @blur="validate('email')"
      />
      <InputError :message="errors.email" />
    </div>
    <p v-if="withPasskey" class="text-sm text-muted-foreground">
      {{ t("We'll mail you a link: it makes your passkey, and your account with it, with no password.") }}
    </p>
    <template v-else>
      <div class="grid gap-2">
        <Label for="password">{{ t('Password') }}</Label>
        <PasswordInput
          id="password"
          name="password"
          autocomplete="new-password"
          required
          :aria-invalid="invalid('password')"
          @blur="validate('password')"
        />
        <InputError :message="errors.password" />
      </div>
      <div class="grid gap-2">
        <Label for="password_confirmation">{{ t('Password again') }}</Label>
        <PasswordInput
          id="password_confirmation"
          name="password_confirmation"
          autocomplete="new-password"
          required
          :aria-invalid="invalid('password_confirmation')"
          @blur="validate('password_confirmation')"
        />
        <InputError :message="errors.password_confirmation" />
      </div>
    </template>
    <Button type="submit" class="w-full" :disabled="processing">
      <LoaderCircle v-if="processing" class="animate-spin" />{{ withPasskey ? t('Mail me a link') : t('Register') }}
    </Button>
  </RegisterForm>
  <Button v-if="works" type="button" variant="outline" class="mt-4 w-full" @click="withPasskey = !withPasskey">
    <LockKeyhole v-if="withPasskey" /><KeyRound v-else />{{ withPasskey ? t('Use a password instead') : t('Use a passkey instead') }}
  </Button>
  <p class="mt-6 text-center text-sm text-muted-foreground">{{ t('Have an account?') }} <TextLink :href="route('login')">{{ t('Log in') }}</TextLink></p>
</template>
