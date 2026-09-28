<script setup lang="ts">
import { Link } from '@inertiajs/vue3'
import { LogOut, Settings } from '@lucide/vue'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { User } from '@/tug/pages'
import { route } from '@/tug/routes'

// UserMenu is who's logged in, in the header, and what's theirs to do:
// their settings, and logging out.
defineProps<{ user: User }>()

// initials are the first letters of a name's first and last words: "AL"
// for Ann Lee.
function initials(name: string) {
  const words = name.trim().split(/\s+/)
  const first = words[0]?.charAt(0) ?? ''
  const last = words.length > 1 ? words[words.length - 1].charAt(0) : ''
  return (first + last).toUpperCase()
}
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" class="h-9 gap-2 px-2" aria-label="Your account">
        <Avatar class="size-7">
          <AvatarFallback class="text-xs font-medium">{{ initials(user.name) }}</AvatarFallback>
        </Avatar>
        <span class="hidden max-w-40 truncate sm:inline">{{ user.name }}</span>
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="w-60">
      <DropdownMenuLabel class="grid font-normal">
        <span class="truncate font-medium">{{ user.name }}</span>
        <span class="truncate text-xs text-muted-foreground">{{ user.email }}</span>
      </DropdownMenuLabel>
      <DropdownMenuSeparator />
      <DropdownMenuItem as-child>
        <Link :href="route('profile.edit')" class="w-full cursor-pointer"><Settings /> Settings</Link>
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem as-child>
        <Link :href="route('logout')" method="post" as="button" class="w-full cursor-pointer"><LogOut /> Log out</Link>
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
