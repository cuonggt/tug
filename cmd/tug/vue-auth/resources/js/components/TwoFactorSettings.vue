<script setup lang="ts">
import { Form, router, usePage } from '@inertiajs/vue3'
import { LoaderCircle, ShieldCheck } from '@lucide/vue'
import { QrcodeSvg } from 'qrcode.vue'
import { computed, ref, watch } from 'vue'
import Heading from '@/components/Heading.vue'
import InputError from '@/components/InputError.vue'
import RecoveryCodes from '@/components/RecoveryCodes.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { vFocus } from '@/lib/focus'
import type { Pages, SharedProps } from '@/tug/pages'
import { form, route } from '@/tug/routes'

// TwoFactorSettings turns two-factor logins on and off: off, a button to
// start; starting, a QR code for the user's authenticator app and a code
// back from it; on, their recovery codes, and a button to stop. The
// handlers are in twofactor.go.
const props = defineProps<Pages['Settings/Security'] & SharedProps>()
const page = usePage()
// The codes come once in the flash as two-factor logins are turned on, or
// new ones are made, and the next visit empties it, as the bell's reload
// does when a notification rings it: kept here, they're shown until the
// page is left. A partial reload that asks for them brings them again.
const kept = ref<string[]>()
watch(
  () => page.flash.recoveryCodes,
  (flashed) => {
    if (flashed) kept.value = flashed
  },
  { immediate: true },
)
const codes = computed(() => page.flash.recoveryCodes ?? kept.value ?? props.recoveryCodes)
// The secret, in fours, to type in by hand.
const key = computed(() => props.setup?.secret.match(/.{1,4}/g)?.join(' '))
</script>

<template>
  <section class="space-y-6">
    <div class="flex items-start justify-between gap-4">
      <Heading
        small
        title="Two-factor logins"
        description="A code from an app on your phone, as well as your password, each time you log in."
      />
      <Badge as="span" :variant="user.twoFactor ? 'default' : 'secondary'">{{ user.twoFactor ? 'On' : 'Off' }}</Badge>
    </div>

    <template v-if="user.twoFactor">
      <RecoveryCodes v-if="codes" :codes="codes" />
      <div v-else class="space-y-3">
        <p class="text-sm text-muted-foreground">Recovery codes log you in when your phone is lost: one code, one login.</p>
        <Button variant="outline" @click="router.reload({ only: ['recoveryCodes'] })">Show my recovery codes</Button>
      </div>
      <Form v-slot="{ processing }" :action="form('two-factor.disable')" :options="{ preserveScroll: true }">
        <Button type="submit" variant="destructive" :disabled="processing">Turn two-factor logins off</Button>
      </Form>
    </template>
    <div v-else-if="setup" class="space-y-6">
      <ol class="list-decimal space-y-2 pl-5 text-sm text-muted-foreground">
        <li>Scan this with an authenticator app, such as 1Password, Google Authenticator or Authy.</li>
        <li>Type the code it shows, to be sure it's set up.</li>
      </ol>
      <div class="flex flex-col items-start gap-4 sm:flex-row sm:items-center">
        <div class="rounded-lg border bg-white p-3">
          <QrcodeSvg :value="setup.url" :size="160" :margin="0" aria-label="The QR code for your authenticator app" />
        </div>
        <div class="space-y-1 text-sm">
          <p class="text-muted-foreground">Can't scan it? Type this key into the app instead:</p>
          <p class="font-mono text-base tracking-wider break-all select-all">{{ key }}</p>
        </div>
      </div>
      <Form
        v-slot="{ errors, processing }"
        :action="form('two-factor.confirm')"
        reset-on-error
        :options="{ preserveScroll: true }"
        class="space-y-4"
      >
        <div class="grid max-w-48 gap-2">
          <Label for="code">The code from the app</Label>
          <Input
            id="code"
            v-focus
            name="code"
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="7"
            placeholder="123456"
            required
            class="font-mono tracking-widest"
            :aria-invalid="!!errors.code"
          />
        </div>
        <InputError :message="errors.code" />
        <div class="flex gap-2">
          <Button type="submit" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />Turn on</Button>
          <Button type="button" variant="ghost" @click="router.delete(route('two-factor.disable'), { preserveScroll: true })">Cancel</Button>
        </div>
      </Form>
    </div>
    <Form v-else v-slot="{ processing }" :action="form('two-factor.enable')" :options="{ preserveScroll: true }">
      <Button type="submit" :disabled="processing"><ShieldCheck />Turn two-factor logins on</Button>
    </Form>
  </section>
</template>
