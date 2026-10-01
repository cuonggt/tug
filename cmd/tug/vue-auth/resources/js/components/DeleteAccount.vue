<script setup lang="ts">
import { Form } from '@inertiajs/vue3'
import { useTemplateRef } from 'vue'
import Heading from '@/components/Heading.vue'
import InputError from '@/components/InputError.vue'
import PasswordInput from '@/components/PasswordInput.vue'
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
import { Label } from '@/components/ui/label'
import { form } from '@/tug/routes'

// DeleteAccount deletes the user's account, after asking for the password
// once more: it can't be undone.
const field = useTemplateRef('password')
</script>

<template>
  <section class="space-y-6">
    <Heading small title="Delete your account" description="Your account, and everything in it, for good." />
    <div class="space-y-4 rounded-lg border border-destructive/30 bg-destructive/5 p-4">
      <p class="text-sm text-destructive">Once it's gone, it's gone: there's no undoing it.</p>
      <Dialog>
        <DialogTrigger as-child>
          <Button variant="destructive">Delete my account</Button>
        </DialogTrigger>
        <DialogContent>
          <DialogTitle>Delete your account?</DialogTitle>
          <DialogDescription>Everything in it goes too, and you're logged out everywhere. Type your password to say you mean it.</DialogDescription>
          <Form
            v-slot="{ errors, processing, resetAndClearErrors }"
            :action="form('profile.destroy')"
            :options="{ preserveScroll: true }"
            reset-on-error
            class="space-y-6"
            @error="field?.focus()"
          >
            <div class="grid gap-2">
              <Label for="delete-password" class="sr-only">Password</Label>
              <PasswordInput
                id="delete-password"
                ref="password"
                name="password"
                autocomplete="current-password"
                placeholder="Password"
              />
              <InputError :message="errors.password" />
            </div>
            <DialogFooter class="gap-2">
              <DialogClose as-child>
                <Button type="button" variant="secondary" @click="resetAndClearErrors()">Keep it</Button>
              </DialogClose>
              <Button type="submit" variant="destructive" :disabled="processing">Delete my account</Button>
            </DialogFooter>
          </Form>
        </DialogContent>
      </Dialog>
    </div>
  </section>
</template>
