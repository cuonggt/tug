<script setup lang="ts">
import { Link } from '@inertiajs/vue3'
import { LogOut, Settings } from '@lucide/vue'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { initials } from '@/lib/utils'
import type { User } from '@/tug/pages'
import { route } from '@/tug/routes'

// UserMenu is who's logged in, in the header, and what's theirs to do:
// their settings, and logging out.
defineProps<{ user: User }>()
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" class="h-9 gap-2 px-2" aria-label="Your account">
        <!-- A new photo, or none, is a new avatar: one keeps the image it loaded. -->
        <Avatar :key="user.photo ?? ''" class="size-7">
          <AvatarImage v-if="user.photo" :src="user.photo" alt="" />
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
