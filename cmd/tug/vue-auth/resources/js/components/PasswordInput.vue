<script setup lang="ts">
import { Eye, EyeOff } from '@lucide/vue'
import { type HTMLAttributes, ref, useTemplateRef } from 'vue'
import { Input } from '@/components/ui/input'
import { useLang } from '@/lib/lang'
import { cn } from '@/lib/utils'

// The page says its own words in the request's language.
const { t } = useLang()

// PasswordInput is a password field with a button that shows what's been
// typed, to check it on a phone's keyboard. What it's given, as its id and
// name, is the field's.
defineOptions({ inheritAttrs: false })
const props = defineProps<{ class?: HTMLAttributes['class'] }>()
const shown = ref(false)

// focus puts the cursor in the field, as deleting the account does when
// the password is wrong.
const field = useTemplateRef('input')
defineExpose({ focus: () => field.value?.$el.focus() })
</script>

<template>
  <div class="relative">
    <Input ref="input" v-bind="$attrs" :type="shown ? 'text' : 'password'" :class="cn('pe-10', props.class)" />
    <button
      type="button"
      :aria-label="shown ? t('Hide the password') : t('Show the password')"
      tabindex="-1"
      class="absolute inset-y-0 end-0 flex items-center rounded-e-md px-3 text-muted-foreground hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
      @click="shown = !shown"
    >
      <EyeOff v-if="shown" class="size-4" />
      <Eye v-else class="size-4" />
    </button>
  </div>
</template>
