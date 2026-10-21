<script setup lang="ts">
import { Head, Link } from '@inertiajs/vue3'
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useLang } from '@/lib/lang'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// The page says its own words in the request's language.
const { t, d } = useLang()

// Dashboard is for users who've verified their email: its route in main.go
// wraps it in usersOnly and verified, and its handler is handed the user.
const props = defineProps<Pages['Dashboard'] & SharedProps>()

// The date is in the page's own language, and in the reader's own zone
// once the page is in their browser.
const since = computed(() => d(props.user.createdAt, { dateStyle: 'long' }))
const code = 'font-mono text-foreground'
</script>

<template>
  <Head :title="t('Dashboard')" />
  <div class="space-y-8">
    <div class="space-y-1">
      <h1 class="text-2xl font-semibold tracking-tight">{{ t('Hello, :name', { name: user.name }) }}</h1>
      <p class="text-muted-foreground">{{ t("You've had an account since :date.", { date: since }) }}</p>
    </div>
    <div class="grid gap-4 md:grid-cols-3">
      <Card>
        <CardHeader>
          <CardTitle>{{ t('Your app goes here') }}</CardTitle>
          <CardDescription>This page is <code :class="code">resources/js/pages/Dashboard.vue</code>, and its handler is
            <code :class="code">dashboard</code> in <code :class="code">main.go</code>.</CardDescription>
        </CardHeader>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{{ t('Add a page') }}</CardTitle>
          <CardDescription>Declare it in Go with <code :class="code">tug.Page[Props]("Name")</code>, route it, and write
            <code :class="code">pages/Name.vue</code>. tug dev writes the types of its props.</CardDescription>
        </CardHeader>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{{ t('Your account') }}</CardTitle>
          <CardDescription>{{ user.twoFactor ? t('Two-factor logins are on.') : t('Two-factor logins are off.') }}
            {{ t('Your password, profile and appearance are in your settings too.') }}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline" size="sm" as-child>
            <Link :href="route('security.edit')">{{ t('Security settings') }}</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
