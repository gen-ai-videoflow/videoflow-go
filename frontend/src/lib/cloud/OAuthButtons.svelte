<script lang="ts">
 import {onMount} from 'svelte';
 import {Button} from '$lib/components/ui/button';
 import {LoaderCircle,Link} from '@lucide/svelte';
 import {request} from './api';
 let {link=false,password='',code=''}:{link?:boolean;password?:string;code?:string}=$props();
 let providers=$state<string[]>([]),linked=$state<string[]>([]),busy=$state(''),error=$state('');
 onMount(()=>{void request('/auth/providers').then(v=>{providers=v.providers||[];linked=v.linked||[];}).catch(()=>{});});
 async function start(provider:string){busy=provider;error='';try{const result=await request(`/auth/oauth/${provider}/start`,{link,...(link?{password,code}:{})});window.location.assign(result.url);}catch(e){error=(e as Error).message;busy='';}}
</script>
{#if providers.length}<div class="oauth-options">{#if link}<p class="text-sm text-muted-foreground">Confirm your identity above, then link a provider you control. An existing account with the same email is never merged automatically.</p>{/if}{#each providers as provider}<Button type="button" variant="outline" disabled={!!busy||(link&&(!password||linked.includes(provider)))} onclick={()=>start(provider)}>{#if busy===provider}<LoaderCircle class="size-4 animate-spin"/>{:else if link}<Link class="size-4"/>{/if}{link?(linked.includes(provider)?'Connected to':'Link'):'Continue with'} {provider==='google'?'Google':'Apple'}</Button>{/each}{#if error}<p class="cloud-alert" role="alert">{error}</p>{/if}</div>{:else if link}<p class="text-sm text-muted-foreground">Additional sign-in providers are not enabled on this deployment.</p>{/if}
<style>.oauth-options{display:grid;gap:10px}.oauth-options p{font-size:12px;line-height:1.7;margin-bottom:6px}</style>
