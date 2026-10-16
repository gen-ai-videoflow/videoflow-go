<script lang="ts">
 import BrandLogo from '$lib/components/BrandLogo.svelte';
 import AuthArtworkCarousel from './AuthArtworkCarousel.svelte';
  import type { Snippet } from 'svelte';
  let {children}:{children:Snippet}=$props();
</script>
<div class="grid min-h-svh bg-background text-foreground lg:grid-cols-2">
  <div class="flex flex-col gap-4 p-6 md:p-10">
    <div class="flex justify-center gap-2 md:justify-start"><a href="/discover" class="flex items-center gap-2 font-medium"><BrandLogo/></a></div>
    <div class="flex flex-1 items-center justify-center py-8"><div class="w-full max-w-xs">{@render children()}</div></div>
  </div>
  <div class="relative hidden bg-muted lg:block"><AuthArtworkCarousel/></div>
</div>
