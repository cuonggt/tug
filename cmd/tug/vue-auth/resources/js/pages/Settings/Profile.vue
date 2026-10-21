<script setup lang="ts">
import { createForm, Head, Link, router } from '@inertiajs/vue3'
import DeleteAccount from '@/components/DeleteAccount.vue'
import Heading from '@/components/Heading.vue'
import InputError from '@/components/InputError.vue'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
  import { useLang } from '@/lib/lang'
import { initials } from '@/lib/utils'
import type { Pages, SharedProps } from '@/tug/pages'
import { form, route, type Inputs } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

// Profile changes the user's name and email, and their photo. A new email
// is mailed a link, and isn't verified until it's followed: updateProfile
// in settings.go. The photo goes up as the form's file, with its progress
// shown, and replaces the one before: updatePhoto in photos.go.
defineProps<Pages['Settings/Profile'] & SharedProps>()

// The forms, typed by their routes' inputs, as tug gen writes them from
// the Go: their errors are the fields they send.
const ProfileForm = createForm<Inputs['profile.update']>()
const PhotoForm = createForm<Inputs['profile.photo.update']>()
</script>

<template>
  <Head :title="t('Profile')" />
  <section class="space-y-6">
    <Heading small :title="t('Profile')" :description="t('Your name, and the email we reach you at.')" />
    <ProfileForm
      v-slot="{ errors, processing }"
      :action="form('profile.update')"
      :options="{ preserveScroll: true }"
      class="space-y-6"
    >
      <div class="grid gap-2">
        <Label for="name">{{ t('Name') }}</Label>
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
        <Label for="email">{{ t('Email') }}</Label>
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
        <p v-if="!user.emailVerifiedAt" class="text-sm text-muted-foreground">{{ t("This email isn't verified yet.") }}
          <Link
            :href="route('verification.send')"
            method="post"
            as="button"
            preserve-scroll
            class="cursor-pointer text-foreground underline underline-offset-4"
          >{{ t('Send the link again') }}</Link></p>
      </div>
      <Button type="submit" :disabled="processing">{{ t('Save') }}</Button>
    </ProfileForm>
  </section>
  <section class="space-y-6">
    <Heading small :title="t('Photo')" :description="t('Shown in place of your initials: a PNG, JPEG or WebP, of 2 MB at most.')" />
    <div class="flex items-start gap-6">
      <Avatar :key="user.photo ?? ''" class="size-16">
        <AvatarImage v-if="user.photo" :src="user.photo" :alt="t('Your photo')" />
        <AvatarFallback class="text-lg font-medium">{{ initials(user.name) }}</AvatarFallback>
      </Avatar>
      <PhotoForm
        v-slot="{ errors, processing, progress }"
        :action="form('profile.photo.update')"
        :options="{ preserveScroll: true }"
        reset-on-success
        class="grid flex-1 gap-2"
      >
        <Label for="photo">{{ t('Choose a photo') }}</Label>
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
          :aria-label="t('Uploading')"
          class="w-full accent-primary"
        />
        <div class="flex gap-2">
          <Button type="submit" :disabled="processing">{{ t('Upload') }}</Button>
          <Button
            v-if="user.photo"
            type="button"
            variant="outline"
            @click="router.delete(route('profile.photo.destroy'), { preserveScroll: true })"
          >{{ t('Remove') }}</Button>
        </div>
      </PhotoForm>
    </div>
  </section>
  <DeleteAccount />
</template>
