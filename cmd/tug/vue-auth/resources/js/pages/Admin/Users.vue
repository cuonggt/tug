<script setup lang="ts">
import { Head, Link, router } from '@inertiajs/vue3'
import { Search } from '@lucide/vue'
import { ref } from 'vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// Users is the admins' page of the users, the newest first, a page at a
// time, found by their email or name: whether each has verified their
// email, turned two-factor logins on, is an admin, or suspended, and when
// they joined; and the buttons that suspend one, restore them, or act as
// them, to see the app as they do. Its handlers are in admin.go, and who
// may do what to whom in abilities.go.
const props = defineProps<Pages['Admin/Users'] & SharedProps>()

const find = ref(props.search)
const submit = () => router.get(route('users.index'), find.value ? { search: find.value } : {}, { preserveState: true })

// when is a time as the page shows it, the same on the server as in any
// browser, whatever its language or zone.
function when(at: string): string {
  return `${new Date(at).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' })} UTC`
}
</script>

<template>
  <Head title="Users" />
  <div class="space-y-8">
    <div class="flex flex-wrap items-end justify-between gap-4">
      <div class="space-y-1">
        <h1 class="text-2xl font-semibold tracking-tight">Users</h1>
        <p class="text-muted-foreground">Everyone with an account, the newest first.</p>
      </div>
      <form role="search" class="flex gap-2" @submit.prevent="submit">
        <Input v-model="find" type="search" placeholder="Email or name" aria-label="Email or name" class="w-56" />
        <Button type="submit" variant="outline">
          <Search />
          Find
        </Button>
      </form>
    </div>
    <p v-if="list.data.length === 0" class="text-muted-foreground">
      {{ search ? `No one's email or name has “${search}” in it.` : 'No one has an account yet.' }}
    </p>
    <ul v-else class="divide-y rounded-lg border">
      <li v-for="user in list.data" :key="user.id" class="flex flex-wrap items-center gap-3 p-4">
        <div class="min-w-0 flex-1 text-sm">
          <p class="flex flex-wrap items-center gap-2 font-medium">
            <span class="truncate">{{ user.name }}</span>
            <!-- Spans, as shadcn's React Badge is: a div can't be in a p. -->
            <Badge v-if="user.admin" as="span" variant="secondary">Admin</Badge>
            <Badge v-if="user.suspendedAt" as="span" variant="destructive">Suspended</Badge>
            <Badge v-if="!user.verified" as="span" variant="outline">Email not verified</Badge>
            <Badge v-if="user.twoFactor" as="span" variant="outline">Two-factor</Badge>
          </p>
          <p class="truncate text-muted-foreground">{{ user.email }}</p>
          <p class="text-xs text-muted-foreground">
            Joined {{ when(user.createdAt) }}<template v-if="user.suspendedAt">, suspended {{ when(user.suspendedAt) }}</template>
          </p>
        </div>
        <div class="flex gap-2">
          <Button
            v-if="user.can.actAs"
            variant="outline"
            size="sm"
            :aria-label="`Act as ${user.name}`"
            @click="router.post(route('users.act', { id: user.id }))"
          >
            Act as
          </Button>
          <Button
            v-if="user.suspendedAt"
            variant="outline"
            size="sm"
            :aria-label="`Restore ${user.name}`"
            @click="router.post(route('users.restore', { id: user.id }), {}, { preserveScroll: true })"
          >
            Restore
          </Button>
          <Button
            v-else-if="user.can.suspend"
            variant="destructive"
            size="sm"
            :aria-label="`Suspend ${user.name}`"
            @click="router.post(route('users.suspend', { id: user.id }), {}, { preserveScroll: true })"
          >
            Suspend
          </Button>
        </div>
      </li>
    </ul>
    <nav v-if="list.prev_page_url || list.next_page_url" aria-label="Pages" class="flex justify-between">
      <Button v-if="list.prev_page_url" variant="outline" size="sm" as-child>
        <Link :href="list.prev_page_url">Newer</Link>
      </Button>
      <span v-else />
      <Button v-if="list.next_page_url" variant="outline" size="sm" as-child>
        <Link :href="list.next_page_url">Older</Link>
      </Button>
    </nav>
  </div>
</template>
