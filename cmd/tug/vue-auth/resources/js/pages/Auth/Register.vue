<script setup lang="ts">
import type { FormComponentRef } from '@inertiajs/core'
import { Form, Head } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import { onBeforeUnmount, useTemplateRef } from 'vue'
import InputError from '@/components/InputError.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { vFocus } from '@/lib/focus'
import { form, route } from '@/tug/routes'

// Register checks each field with the server as it's left, as BindValid in
// auth.go answers: a taken email shows before the form is sent.
defineOptions({
  layout: { title: 'Make an account', description: 'Your name, your email, and a password of 8 characters or more.' },
})

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
</script>

<template>
  <Head title="Register" />
  <Form
    ref="registration"
    v-slot="{ errors, processing, validate, invalid }"
    :action="form('register.store')"
    :reset-on-error="['password', 'password_confirmation']"
    :validation-timeout="300"
    class="flex flex-col gap-6"
    @start="dropCheck"
  >
    <div class="grid gap-2">
      <Label for="name">Name</Label>
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
      <Label for="email">Email</Label>
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
    <div class="grid gap-2">
      <Label for="password">Password</Label>
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
      <Label for="password_confirmation">Password again</Label>
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
    <Button type="submit" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />Register</Button>
  </Form>
  <p class="mt-6 text-center text-sm text-muted-foreground">Have an account? <TextLink :href="route('login')">Log in</TextLink></p>
</template>
