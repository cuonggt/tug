<script setup lang="ts">
import { createForm, router, usePage } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import { ref, useTemplateRef } from 'vue'
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
  import { useLang } from '@/lib/lang'
import { dismissed, getPasskey, optionsFrom } from '@/lib/passkeys'
import { form, route, type Inputs } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

// DeleteAccount deletes the user's account, after asking for the password
// once more, or for an account with none, a passkey: it can't be undone.
const page = usePage()
const field = useTemplateRef('password')
const deleting = ref(false)
const failure = ref<string>()

const withPasskey = async () => {
  deleting.value = true
  failure.value = undefined
  try {
    const options = await optionsFrom(route('password.confirm.passkey.options'))
    if (!options) return
    const credential = await getPasskey(options)
    router.delete(route('profile.destroy'), { data: { credential }, preserveScroll: true, onFinish: () => (deleting.value = false) })
  } catch (err) {
    deleting.value = false
    if (!dismissed(err)) failure.value = t("Your browser didn't use a passkey: try again.")
  }
}

// The form, typed by its route's input, as tug gen writes it from the Go:
// its errors are the fields it sends.
const DeleteForm = createForm<Inputs['profile.destroy']>()
</script>

<template>
  <section class="space-y-6">
    <Heading small :title="t('Delete your account')" :description="t('Your account, and everything in it, for good.')" />
    <div class="space-y-4 rounded-lg border border-destructive/30 bg-destructive/5 p-4">
      <p class="text-sm text-destructive">{{ t("Once it's gone, it's gone: there's no undoing it.") }}</p>
      <Dialog>
        <DialogTrigger as-child>
          <Button variant="destructive">{{ t('Delete my account') }}</Button>
        </DialogTrigger>
        <DialogContent>
          <DialogTitle>{{ t('Delete your account?') }}</DialogTitle>
          <template v-if="page.props.auth.user?.password">
            <DialogDescription>{{ t("Everything in it goes too, and you're logged out everywhere. Type your password to say you mean it.") }}</DialogDescription>
            <DeleteForm
              v-slot="{ errors, processing, resetAndClearErrors }"
              :action="form('profile.destroy')"
              :options="{ preserveScroll: true }"
              reset-on-error
              class="space-y-6"
              @error="field?.focus()"
            >
              <div class="grid gap-2">
                <Label for="delete-password" class="sr-only">{{ t('Password') }}</Label>
                <PasswordInput
                  id="delete-password"
                  ref="password"
                  name="password"
                  autocomplete="current-password"
                  :placeholder="t('Password')"
                />
                <InputError :message="errors.password" />
              </div>
              <DialogFooter class="gap-2">
                <DialogClose as-child>
                  <Button type="button" variant="secondary" @click="resetAndClearErrors()">{{ t('Keep it') }}</Button>
                </DialogClose>
                <Button type="submit" variant="destructive" :disabled="processing">{{ t('Delete my account') }}</Button>
              </DialogFooter>
            </DeleteForm>
          </template>
          <template v-else>
            <DialogDescription>{{ t("Everything in it goes too, and you're logged out everywhere. Use your passkey to say you mean it.") }}</DialogDescription>
            <InputError :message="page.props.errors.passkey ?? failure" />
            <DialogFooter class="gap-2">
              <DialogClose as-child>
                <Button type="button" variant="secondary">{{ t('Keep it') }}</Button>
              </DialogClose>
              <Button type="button" variant="destructive" :disabled="deleting" @click="withPasskey">
                <LoaderCircle v-if="deleting" class="animate-spin" />{{ t('Delete my account') }}
              </Button>
            </DialogFooter>
          </template>
        </DialogContent>
      </Dialog>
    </div>
  </section>
</template>
