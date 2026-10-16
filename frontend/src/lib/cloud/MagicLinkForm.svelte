<script lang="ts">
 import {onMount} from 'svelte';
 import {page} from '$app/state';
 import {Button} from '$lib/components/ui/button';
 import {Input} from '$lib/components/ui/input';
 import {Label} from '$lib/components/ui/label';
 import {LoaderCircle,Mail,KeyRound,ArrowLeft} from '@lucide/svelte';
 import AuthFrame from './AuthFrame.svelte';
 import {authHref} from './auth';
 import {request} from './api';
 import {usePasskey} from './webauthn';
 let {returnTo='/discover',onSignedIn}:{returnTo?:string;onSignedIn:()=>Promise<void>}=$props();
 let email=$state(''),token=$state(''),challenge=$state(''),methods=$state<string[]>([]),code=$state(''),busy=$state(false),error=$state(''),notice=$state(''),developmentToken=$state('');
 onMount(()=>{token=page.url.searchParams.get('token')||'';challenge=page.url.searchParams.get('challenge')||'';if(challenge)methods=['totp','recovery','passkey'];});
 async function action(work:()=>Promise<void>){busy=true;error='';try{await work();}catch(e){error=(e as Error).message;}finally{busy=false;}}
 async function send(){await action(async()=>{const result=await request('/auth/magic-link',{email:email.trim()});notice=result.message;developmentToken=result.development_token||'';});}
 async function verify(){await action(async()=>{const result=await request('/auth/magic-link/verify',{token});token='';if(result.mfa_required){challenge=result.challenge;methods=result.methods||[];notice='Email verified. Complete your usual second-factor verification.';}else await onSignedIn();});}
 async function verifyCode(){await action(async()=>{await request('/auth/mfa',{challenge,code:code.trim()});code='';await onSignedIn();});}
 async function verifyPasskey(){await action(async()=>{const begin=await request('/auth/passkeys/options',{});const credential=await usePasskey(begin.options);await request('/auth/passkeys/verify',{challenge:begin.challenge,credential});await onSignedIn();});}
</script>
<svelte:head><meta name="referrer" content="no-referrer"/></svelte:head>
<AuthFrame><div class="flex flex-col gap-6"><div class="text-center space-y-2"><Mail class="size-6 mx-auto"/><h1 class="text-2xl font-bold">{challenge?'Verify your sign-in':token?'Your sign-in link is ready':'Sign in with email'}</h1><p class="text-sm text-muted-foreground">{challenge?'Your account’s extra protection remains enabled.':token?'Continue only if you requested this link. It can be used once.':'We’ll email a private sign-in link. Your account’s authenticator and passkey protections still apply.'}</p></div>
{#if error}<p class="cloud-alert" role="alert">{error}</p>{/if}{#if notice}<p class="cloud-notice" role="status">{notice}</p>{/if}
{#if challenge}{#if methods.includes('totp')||methods.includes('recovery')}<form class="grid gap-3" onsubmit={e=>{e.preventDefault();void verifyCode();}}><Label for="magic-mfa">Authenticator or recovery code</Label><Input id="magic-mfa" bind:value={code} autocomplete="one-time-code" required disabled={busy}/><Button type="submit" disabled={busy||!code.trim()}>Verify and sign in</Button></form>{/if}{#if methods.includes('passkey')}<Button variant="outline" disabled={busy} onclick={verifyPasskey}><KeyRound class="size-4"/>Verify with passkey</Button>{/if}
{:else if token}<Button disabled={busy} onclick={verify}>{#if busy}<LoaderCircle class="size-4 animate-spin"/>{/if}Continue to VideoFlow</Button>
{:else}<form class="grid gap-3" onsubmit={e=>{e.preventDefault();void send();}}><Label for="magic-email">Account email</Label><Input id="magic-email" type="email" bind:value={email} autocomplete="email" required disabled={busy}/><Button type="submit" disabled={busy}>{#if busy}<LoaderCircle class="size-4 animate-spin"/>{/if}{notice?'Send a new sign-in link':'Email me a sign-in link'}</Button><p class="text-xs text-muted-foreground">Links expire in 15 minutes. Requesting a new link replaces your previous link.</p></form>{/if}
{#if developmentToken}<div class="rounded-md border p-3 text-xs"><p class="mb-2">Local development only</p><a class="underline" href={`/magic-link?token=${encodeURIComponent(developmentToken)}&returnTo=${encodeURIComponent(returnTo)}`}>Open local sign-in link</a></div>{/if}
<a class="text-center text-sm underline underline-offset-4" href={authHref('/login',returnTo,page.url.origin)}><ArrowLeft class="size-4 inline"/> Back to password sign-in</a></div></AuthFrame>
