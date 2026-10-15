<script lang="ts">
 import {goto} from '$app/navigation';
 import type {Snippet} from 'svelte';
 import * as Dropdown from '$lib/components/ui/dropdown-menu';
 import {Button} from '$lib/components/ui/button';
 import {UserRound,ChevronDown,Clapperboard,LogOut,Search} from '@lucide/svelte';
 let {user,path,signInHref,onSignOut,children}:{user:any;path:string;signInHref:string;onSignOut:()=>void;children:Snippet}=$props();
</script>
<div class="cloud-app viewer-shell flex h-screen min-w-0 flex-col bg-background text-foreground">
 <header class="viewer-header"><a href="/" class="text-sm font-semibold tracking-wide">VideoFlow</a><nav aria-label="Viewing navigation"><a href="/discover" aria-current={['/','/discover'].includes(path)?'page':undefined}>Discover</a><a href="/following" aria-current={path==='/following'?'page':undefined}>Following</a><a href="/search" aria-current={path==='/search'?'page':undefined}><Search class="size-4"/>Search</a><a href="/premium" aria-current={path==='/premium'?'page':undefined}>Premium</a></nav><div class="viewer-account">{#if user}<Dropdown.Root><Dropdown.Trigger class="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm hover:bg-accent"><UserRound class="size-4"/><span class="max-w-28 truncate">{user.name || user.handle || 'Your account'}</span><ChevronDown class="size-3.5"/></Dropdown.Trigger><Dropdown.Content align="end"><Dropdown.Label>{user.email}</Dropdown.Label><Dropdown.Item onSelect={()=>void goto(`/channels/${user.handle}`)}>Your public profile</Dropdown.Item><Dropdown.Item onSelect={()=>void goto('/profile')}>Edit profile</Dropdown.Item><Dropdown.Item onSelect={()=>void goto('/settings')}>Account settings</Dropdown.Item><Dropdown.Item onSelect={()=>void goto('/security')}>Security</Dropdown.Item><Dropdown.Item onSelect={()=>void goto('/payments')}>Payments</Dropdown.Item><Dropdown.Separator/><Dropdown.Item onSelect={()=>void goto('/studio')}><Clapperboard class="size-4"/>Open Studio</Dropdown.Item><Dropdown.Separator/><Dropdown.Item onSelect={onSignOut}><LogOut class="size-4"/>Sign out</Dropdown.Item></Dropdown.Content></Dropdown.Root>{:else}<Button href={signInHref} size="sm">Sign in</Button>{/if}</div></header>
 {@render children()}
</div>
