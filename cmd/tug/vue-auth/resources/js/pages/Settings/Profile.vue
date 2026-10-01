<script setup lang="ts">
import { Form, Head, Link, router } from '@inertiajs/vue3'
import DeleteAccount from '@/components/DeleteAccount.vue'
import Heading from '@/components/Heading.vue'
import InputError from '@/components/InputError.vue'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { initials } from '@/lib/utils'
import type { Pages, SharedProps } from '@/tug/pages'
import { form, route } from '@/tug/routes'

// Profile changes the user's name and email, and their photo. A new email
// is mailed a link, and isn't verified until it's followed: updateProfile
// in settings.go. The photo goes up as the form's file, with its progress
// shown, and replaces the one before: updatePhoto in photos.go.
defineProps<Pages['Settings/Profile'] & SharedProps>()
</script>

<template>
  <Head title="Profile" />
  <section class="space-y-6">
    <Heading small title="Profile" description="Your name, and the email we reach you at." />
    <Form
      v-slot="{ errors, processing }"
      :action="form('profile.update')"
      :options="{ preserveScroll: true }"
      class="space-y-6"
    >
      <div class="grid gap-2">
        <Label for="name">Name</Label>
        <Input
          id="name"
          name="name"
          :default-value="user.name"
          autocomplete="name"
          required
          :aria-invalid="!!errors.name"
        />
        <InputError :message="errors.name" />
      </div>
      <div class="grid gap-2">
        <Label for="email">Email</Label>
        <Input
          id="email"
          name="email"
          type="email"
          :default-value="user.email"
          autocomplete="username"
          required
          :aria-invalid="!!errors.email"
        />
        <InputError :message="errors.email" />
        <p v-if="!user.emailVerifiedAt" class="text-sm text-muted-foreground">This email isn't verified yet.
          <Link
            :href="route('verification.send')"
            method="post"
            as="button"
            preserve-scroll
            class="cursor-pointer text-foreground underline underline-offset-4"
          >Send the link again</Link></p>
      </div>
      <Button type="submit" :disabled="processing">Save</Button>
    </Form>
  </section>
  <section class="space-y-6">
    <Heading small title="Photo" description="Shown in place of your initials: a PNG, JPEG or WebP, of 2 MB at most." />
    <div class="flex items-start gap-6">
      <Avatar :key="user.photo ?? ''" class="size-16">
        <AvatarImage v-if="user.photo" :src="user.photo" alt="Your photo" />
        <AvatarFallback class="text-lg font-medium">{{ initials(user.name) }}</AvatarFallback>
      </Avatar>
      <Form
        v-slot="{ errors, processing, progress }"
        :action="form('profile.photo.update')"
        :options="{ preserveScroll: true }"
        reset-on-success
        class="grid flex-1 gap-2"
      >
        <Label for="photo">Choose a photo</Label>
        <Input
          id="photo"
          name="photo"
          type="file"
          accept="image/png,image/jpeg,image/webp"
          required
          :aria-invalid="!!errors.photo"
        />
        <InputError :message="errors.photo" />
        <progress
          v-if="progress"
          :value="progress.percentage"
          max="100"
          aria-label="Uploading"
          class="w-full accent-primary"
        />
        <div class="flex gap-2">
          <Button type="submit" :disabled="processing">Upload</Button>
          <Button
            v-if="user.photo"
            type="button"
            variant="outline"
            @click="router.delete(route('profile.photo.destroy'), { preserveScroll: true })"
          >Remove</Button>
        </div>
      </Form>
    </div>
  </section>
  <DeleteAccount />
</template>
