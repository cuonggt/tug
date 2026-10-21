<script setup lang="ts">
import { Form, Head, Link, router, usePage } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import { onMounted, onUnmounted } from 'vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
  import { useLang } from '@/lib/lang'
import { listen } from '@/lib/broadcasts'
  import { text } from '@/tug/lang'
import { form, route } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

// VerifyEmail is where the pages for verified users send someone who
// hasn't followed the link mailed to them yet. In development, without
// MAIL_HOST in .env, the mail is kept in the app's mailbox, at /_tug/mail,
// which tug dev's terminal links to.
defineOptions({
  layout: { title: text('Verify your email'), description: text('One more step, and the app is yours to use.') },
})
const page = usePage()

// Verified in another tab, or on the phone the mail went to, the email
// moves this page on: the app says so on the user's own channel, and the
// page reloads, which its handler, in verify.go, answers with the
// dashboard once the email is verified.
let stop: (() => void) | undefined
onMounted(() => (stop = listen('verified', () => router.reload())))
onUnmounted(() => stop?.())
</script>

<template>
  <Head :title="t('Verify your email')" />
  <div class="flex flex-col gap-4 text-center">
    <p class="text-sm text-muted-foreground">{{ t('We mailed a link to') }} <span class="font-medium text-foreground">{{ page.props.auth.user?.email }}</span>{{
      t('. Follow it to verify the email is yours; it works for a day.')
    }}</p>
    <Form v-slot="{ processing }" :action="form('verification.send')">
      <Button type="submit" variant="secondary" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />{{ t('Send another link') }}</Button>
    </Form>
    <p class="text-sm text-muted-foreground">{{ t('The wrong email?') }} <TextLink :href="route('profile.edit')">{{ t('Change it') }}</TextLink>{{ t(', or') }}
      <Link :href="route('logout')" method="post" as="button" class="cursor-pointer underline underline-offset-4">{{ t('log out') }}</Link>.</p>
  </div>
</template>
