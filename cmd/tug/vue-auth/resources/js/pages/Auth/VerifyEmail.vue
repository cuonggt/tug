<script setup lang="ts">
import { Form, Head, Link, usePage } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { route } from '@/tug/routes'

// VerifyEmail is where the pages for verified users send someone who
// hasn't followed the link mailed to them yet. In development, without
// MAIL_HOST in .env, the mail is written to tug dev's terminal.
defineOptions({
  layout: { title: 'Verify your email', description: 'One more step, and the app is yours to use.' },
})
const page = usePage()
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
