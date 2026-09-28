<script setup lang="ts">
import { Form } from '@inertiajs/vue3'
import { Check, Copy } from '@lucide/vue'
import { ref } from 'vue'
import { Button } from '@/components/ui/button'
import { route } from '@/tug/routes'

// RecoveryCodes lists the user's recovery codes, to copy somewhere safe,
// and makes new ones. TwoFactorSettings shows it.
const props = defineProps<{ codes: string[] }>()
const copied = ref(false)
const copy = async () => {
  await navigator.clipboard.writeText(props.codes.join('\n'))
  copied.value = true
  setTimeout(() => (copied.value = false), 2000)
}
</script>

<template>
  <div class="space-y-3">
    <p class="text-sm text-muted-foreground">Keep these somewhere safe, such as a password manager. Each logs you in once, when your phone is lost.</p>
    <ul v-if="codes.length > 0" class="grid grid-cols-2 gap-x-6 gap-y-1 rounded-lg bg-muted p-4 font-mono text-sm">
      <li v-for="code in codes" :key="code">{{ code }}</li>
    </ul>
    <p v-else class="rounded-lg bg-muted p-4 text-sm">You've used them all: make new ones.</p>
    <div class="flex flex-wrap gap-2">
      <Button v-if="codes.length > 0" type="button" variant="outline" @click="copy">
        <Check v-if="copied" />
        <Copy v-else />{{ copied ? 'Copied' : 'Copy' }}
      </Button>
      <Form v-slot="{ processing }" :action="route('two-factor.recovery-codes')" method="post" :options="{ preserveScroll: true }">
        <Button type="submit" variant="outline" :disabled="processing">Make new codes</Button>
      </Form>
    </div>
  </div>
</template>
