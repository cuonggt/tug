<script setup lang="ts">
import { Form, Head, Link, router, usePage } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import { onMounted, onUnmounted } from 'vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { listen } from '@/lib/broadcasts'
import { route } from '@/tug/routes'

// VerifyEmail is where the pages for verified users send someone who
// hasn't followed the link mailed to them yet. In development, without
// MAIL_HOST in .env, the mail is written to tug dev's terminal.
defineOptions({
  layout: { title: 'Verify your email', description: 'One more step, and the app is yours to use.' },
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
  <Head title="Verify your email" />
  <div class="flex flex-col gap-4 text-center">
    <p class="text-sm text-muted-foreground">We mailed a link to <span class="font-medium text-foreground">{{ page.props.auth.user?.email }}</span>. Follow it to
      verify the email is yours; it works for a day.</p>
    <Form v-slot="{ processing }" :action="route('verification.send')" method="post">
      <Button type="submit" variant="secondary" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />Send another link</Button>
    </Form>
    <p class="text-sm text-muted-foreground">The wrong email? <TextLink :href="route('profile.edit')">Change it</TextLink>, or
      <Link :href="route('logout')" method="post" as="button" class="cursor-pointer underline underline-offset-4">log out</Link>.</p>
  </div>
</template>
