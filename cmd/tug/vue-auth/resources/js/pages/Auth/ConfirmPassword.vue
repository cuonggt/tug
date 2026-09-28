<script setup lang="ts">
import { Form, Head, router, usePage } from '@inertiajs/vue3'
import { KeyRound, LoaderCircle } from '@lucide/vue'
import { onMounted, ref } from 'vue'
import InputError from '@/components/InputError.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { vFocus } from '@/lib/focus'
import { dismissed, getPasskey, optionsFrom, passkeysWork } from '@/lib/passkeys'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// ConfirmPassword asks for the password again before the settings that
// could hand the account to someone else: passwordConfirmed in auth.go
// sends users here when they last typed it over three hours ago. One of
// their passkeys says it's them as well.
defineOptions({
  layout: {
    title: 'Confirm your password',
    description: "You're on your way to your account's settings, which ask for it once in a while.",
  },
})
defineProps<Pages['Auth/ConfirmPassword'] & SharedProps>()
const page = usePage()
const works = ref(false)
const failure = ref<string>()
onMounted(() => (works.value = passkeysWork()))

const withPasskey = async () => {
  failure.value = undefined
  try {
    const options = await optionsFrom(route('password.confirm.passkey.options'))
    if (options) router.post(route('password.confirm.passkey'), { credential: await getPasskey(options) })
  } catch (err) {
    if (!dismissed(err)) failure.value = "Your browser didn't use a passkey: try again, or type your password."
  }
}
</script>

<template>
  <Head title="Confirm your password" />
  <Form
    v-slot="{ errors, processing }"
    :action="route('password.confirm.store')"
    method="post"
    reset-on-error
    class="flex flex-col gap-6"
  >
    <div class="grid gap-2">
      <Label for="password">Password</Label>
      <PasswordInput id="password" v-focus name="password" autocomplete="current-password" required />
      <InputError :message="errors.password" />
    </div>
    <Button type="submit" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />Confirm</Button>
  </Form>
  <div v-if="passkeys && works" class="mt-4 grid gap-2">
    <Button type="button" variant="outline" class="w-full" @click="withPasskey"><KeyRound />Use a passkey</Button>
    <InputError :message="page.props.errors.passkey ?? failure" />
  </div>
</template>
