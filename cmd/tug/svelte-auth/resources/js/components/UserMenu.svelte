<script lang="ts">
  import { Link } from '@inertiajs/svelte'
  import LogOut from '@lucide/svelte/icons/log-out'
  import Settings from '@lucide/svelte/icons/settings'
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
  // their settings, and logging out. Its items are Inertia's links, which
  // take the item's props, so they visit as the app's other links do.
  let { user }: { user: User } = $props()
</script>

<DropdownMenu>
  <DropdownMenuTrigger>
    {#snippet child({ props })}
      <Button {...props} variant="ghost" class="h-9 gap-2 px-2" aria-label="Your account">
        <!-- A new photo, or none, is a new avatar: one keeps the image it loaded. -->
        {#key user.photo}
          <Avatar class="size-7">
            {#if user.photo}
              <AvatarImage src={user.photo} alt="" />
            {/if}
            <AvatarFallback class="text-xs font-medium">{initials(user.name)}</AvatarFallback>
          </Avatar>
        {/key}
        <span class="hidden max-w-40 truncate sm:inline">{user.name}</span>
      </Button>
    {/snippet}
  </DropdownMenuTrigger>
  <!-- Named for the button that opens it, as a menu should be: bits-ui
       leaves it unnamed. -->
  <DropdownMenuContent align="end" class="w-60" aria-label="Your account">
    <DropdownMenuLabel class="grid font-normal">
      <span class="truncate font-medium">{user.name}</span>
      <span class="truncate text-xs text-muted-foreground">{user.email}</span>
    </DropdownMenuLabel>
    <DropdownMenuSeparator />
    <DropdownMenuItem class="w-full cursor-pointer">
      {#snippet child({ props })}
        <Link href={route('profile.edit')} {...props}><Settings /> Settings</Link>
      {/snippet}
    </DropdownMenuItem>
    <DropdownMenuSeparator />
    <DropdownMenuItem class="w-full cursor-pointer">
      {#snippet child({ props })}
        <Link href={route('logout')} method="post" as="button" {...props}><LogOut /> Log out</Link>
      {/snippet}
    </DropdownMenuItem>
  </DropdownMenuContent>
</DropdownMenu>
