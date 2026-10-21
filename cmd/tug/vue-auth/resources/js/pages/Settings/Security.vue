<script setup lang="ts">
import { createForm, Head, router } from '@inertiajs/vue3'
import Heading from '@/components/Heading.vue'
import InputError from '@/components/InputError.vue'
import LoginSettings from '@/components/LoginSettings.vue'
import PasskeySettings from '@/components/PasskeySettings.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import TwoFactorSettings from '@/components/TwoFactorSettings.vue'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { useLang } from '@/lib/lang'
import type { Pages, SharedProps } from '@/tug/pages'
import { form, route, type Inputs } from '@/tug/routes'

// The page says its own words in the request's language.
const { t } = useLang()

// Security changes the user's password, which logs them out everywhere
// else, or sets one, for an account made with a passkey, or removes it,
// for one with a passkey to log in with; turns two-factor logins on and
// off, keeps their passkeys, and lists the browsers they're logged in
// from.
const props = defineProps<Pages['Settings/Security'] & SharedProps>()

// The form, typed by its route's input, as tug gen writes it from the Go:
// its errors are the fields it sends.
const PasswordForm = createForm<Inputs['user-password.update']>()
</script>

<template>
  <Head :title="t('Security')" />
  <section class="space-y-6">
    <Heading
      small
      :title="t('Password')"
      :description="
        user.password
          ? t('A new one logs you out everywhere else, such as a lost laptop.') : t('Your account logs in with its passkeys alone. A password logs in too, and logs you out everywhere else.')
      "
    />
    <PasswordForm
      v-slot="{ errors, processing }"
      :action="form('user-password.update')"
      :options="{ preserveScroll: true }"
      reset-on-error
      reset-on-success
      class="space-y-6"
    >
      <div v-if="user.password" class="grid gap-2">
        <Label for="current_password">{{ t('Current password') }}</Label>
        <PasswordInput id="current_password" name="current_password" autocomplete="current-password" required />
        <InputError :message="errors.current_password" />
      </div>
      <div class="grid gap-2">
        <Label for="password">{{ t('New password') }}</Label>
        <PasswordInput id="password" name="password" autocomplete="new-password" required />
        <InputError :message="errors.password" />
      </div>
      <div class="grid gap-2">
        <Label for="password_confirmation">{{ t('New password again') }}</Label>
        <PasswordInput id="password_confirmation" name="password_confirmation" autocomplete="new-password" required />
        <InputError :message="errors.password_confirmation" />
      </div>
      <Button type="submit" :disabled="processing">{{ user.password ? t('Change the password') : t('Set a password') }}</Button>
    </PasswordForm>
    <div v-if="user.password && passkeys.length > 0" class="space-y-2">
      <p class="text-sm text-muted-foreground">{{ t('Your passkeys log you in without it.') }}</p>
      <Button variant="outline" @click="router.delete(route('user-password.destroy'), { preserveScroll: true })">{{ t('Remove the password') }}</Button>
    </div>
  </section>
  <PasskeySettings v-bind="props" />
  <TwoFactorSettings v-bind="props" />
  <LoginSettings v-bind="props" />
</template>
