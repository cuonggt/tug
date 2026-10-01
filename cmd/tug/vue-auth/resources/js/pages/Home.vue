<script setup lang="ts">
import { Head, Link } from '@inertiajs/vue3'
import { Database, KeyRound, LayoutDashboard, Mail, Package, ShieldCheck } from '@lucide/vue'
import ActingBanner from '@/components/ActingBanner.vue'
import AppLogo from '@/components/AppLogo.vue'
import { Button } from '@/components/ui/button'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// what is what the app comes with, and where it is, until this page says
// what the app is for.
const what = [
  { icon: KeyRound, title: 'Accounts', text: 'Registering, logging in, and a forgotten password reset by mail.', where: 'auth.go' },
  { icon: Mail, title: 'Verified email', text: 'A link mailed to each new email, which the dashboard waits for.', where: 'verify.go' },
  { icon: ShieldCheck, title: 'Two-factor logins', text: 'Codes from an authenticator app, and recovery codes.', where: 'twofactor.go' },
  { icon: LayoutDashboard, title: 'Settings', text: 'Profile, email, password, appearance, and deleting the account.', where: 'settings.go' },
  { icon: Database, title: 'SQLite', text: 'Users in database/sql, and migrations that run at start.', where: 'users.go' },
  { icon: Package, title: 'One binary', text: 'tug build puts the frontend inside the Go server.', where: 'Dockerfile' },
]

// Home is the landing page, main.go's home, with no layout around it but
// the line that says an admin is acting as the user. Vue makes a page's
// props from their type, which it can't work out from PageProps<'Home'>,
// so they're spelled out.
defineProps<Pages['Home'] & SharedProps>()
</script>

<template>
  <Head title="Welcome" />
  <div class="flex min-h-svh flex-col">
    <ActingBanner />
    <header class="mx-auto flex h-16 w-full max-w-5xl items-center justify-between px-4">
      <AppLogo />
      <nav class="flex items-center gap-2">
        <Button v-if="auth.user" size="sm" as-child>
          <Link :href="route('dashboard')">Dashboard</Link>
        </Button>
        <template v-else>
          <Button variant="ghost" size="sm" as-child>
            <Link :href="route('login')">Log in</Link>
          </Button>
          <Button size="sm" as-child>
            <Link :href="route('register')">Register</Link>
          </Button>
        </template>
      </nav>
    </header>

    <main class="mx-auto flex w-full max-w-5xl flex-1 flex-col justify-center gap-16 px-4 py-16">
      <div class="max-w-2xl space-y-6">
        <h1 class="text-4xl font-semibold tracking-tight text-balance sm:text-5xl">{{ appName }}</h1>
        <p class="text-lg text-muted-foreground">Go handlers render Vue pages, with Inertia in between and no API to write. People make accounts, verify
          their email, and log in with a second factor if they like.</p>
        <div class="flex gap-3">
          <Button size="lg" as-child>
            <Link :href="auth.user ? route('dashboard') : route('register')">{{ auth.user ? 'Your dashboard' : 'Get started' }}</Link>
          </Button>
        </div>
      </div>

      <ul class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <li v-for="{ icon, title, text, where } in what" :key="title" class="rounded-xl border p-5">
          <component :is="icon" class="size-5 text-muted-foreground" />
          <h2 class="mt-3 font-medium">{{ title }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ text }}</p>
          <code class="mt-3 inline-block rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{{ where }}</code>
        </li>
      </ul>
    </main>

    <footer class="mx-auto w-full max-w-5xl px-4 py-8 text-sm text-muted-foreground">This page is <code class="font-mono">resources/js/pages/Home.vue</code>: make it say what {{ appName }} is for.</footer>
  </div>
</template>
