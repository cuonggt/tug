<script setup lang="ts">
import { Form, Head } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import { ref } from 'vue'
import InputError from '@/components/InputError.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { vFocus } from '@/lib/focus'
import { form } from '@/tug/routes'

// TwoFactorChallenge is the second step of logging in, for a user who has
// turned two-factor logins on: a code from their authenticator app, or,
// with the phone lost, one of their recovery codes.
defineOptions({
  layout: {
    title: 'Two-factor login',
    description: 'The six-digit code your authenticator app shows for this account.',
  },
})
const recovery = ref(false)
</script>

<template>
  <Head title="Two-factor login" />
  <Form
    :key="recovery ? 'recovery' : 'code'"
    v-slot="{ errors, processing }"
    :action="form('two-factor.login.store')"
    reset-on-error
    class="flex flex-col gap-6"
  >
    <div v-if="recovery" class="grid gap-2">
      <Label for="recovery_code">Recovery code</Label>
      <Input
        id="recovery_code"
        v-focus
        name="recovery_code"
        autocomplete="off"
        placeholder="abcde-fghij"
        required
        :aria-invalid="!!errors.recovery_code"
      />
      <InputError :message="errors.recovery_code" />
    </div>
    <div v-else class="grid gap-2">
      <Label for="code">Code</Label>
      <Input
        id="code"
        v-focus
        name="code"
        inputmode="numeric"
        autocomplete="one-time-code"
        pattern="[0-9 ]*"
        maxlength="7"
        placeholder="123456"
        required
        class="text-center font-mono text-lg tracking-[0.3em]"
        :aria-invalid="!!errors.code"
      />
      <InputError :message="errors.code" />
    </div>
    <Button type="submit" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />Log in</Button>
  </Form>
  <p class="mt-6 text-center text-sm text-muted-foreground">
    {{ recovery ? 'Found your phone?' : 'Lost your phone?' }}
    <button type="button" class="cursor-pointer text-foreground underline underline-offset-4" @click="recovery = !recovery">
      {{ recovery ? 'Use a code from the app' : 'Use a recovery code' }}
    </button>
  </p>
</template>
