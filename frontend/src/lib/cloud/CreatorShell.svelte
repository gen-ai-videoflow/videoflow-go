<script lang="ts">
 import BrandLogo from '$lib/components/BrandLogo.svelte';
 import type {Snippet} from 'svelte';
 import {ArrowLeft,Settings} from '@lucide/svelte';
 let {path,children}:{path:string;children:Snippet}=$props();
</script>
<div class="cloud-app creator-shell flex h-screen min-w-0 flex-col bg-background text-foreground">
 <header><a class="studio-link" href="/studio/dashboard"><BrandLogo compact/><span class="studio-label">Studio</span></a><a class="settings-link" href="/settings/account" aria-label="Account settings"><Settings class="size-4"/></a><a href="/discover" class="watch-link"><ArrowLeft class="size-4"/>Watch videos</a></header>
 <nav aria-label="Publishing navigation"><a href="/channel" aria-current={path==='/channel'?'page':undefined}>My content</a><a href="/studio/videos">Finished outputs</a><a href="/studio/upload" aria-current={path==='/studio/upload'?'page':undefined}>Upload video</a></nav>
 {@render children()}
</div>
<style>
header{display:flex;align-items:center;gap:24px;padding:14px 28px;border-bottom:1px solid var(--border);flex-shrink:0;font-size:13px}.studio-label{font-size:11px;color:var(--muted-foreground)}.studio-link{display:flex;gap:9px;align-items:center;font-weight:600;margin-right:auto}.watch-link{display:flex;align-items:center;gap:8px;color:var(--muted-foreground)}.settings-link{display:grid;place-content:center;width:36px;height:36px}nav{display:flex;gap:24px;padding:0 28px;border-bottom:1px solid var(--border);flex-shrink:0}nav a{padding:14px 0;font-size:13px;color:var(--muted-foreground);border-bottom:2px solid transparent}nav a[aria-current=page]{color:var(--foreground);border-color:var(--primary)}a:focus-visible{outline:2px solid var(--ring);outline-offset:4px;border-radius:3px}@media(max-width:600px){header{padding:10px 16px;gap:14px;min-height:60px}.settings-link{width:44px;height:44px}.watch-link{font-size:12px}.watch-link :global(svg){display:none}nav{gap:22px;padding:0 16px}nav a{min-height:44px;font-size:12px}}
</style>
