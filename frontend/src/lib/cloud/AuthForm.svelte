<script lang="ts">
  import { LoaderCircle, KeyRound } from '@lucide/svelte';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import { page } from '$app/state';
  import { authHref } from './auth';
  import AuthFrame from './AuthFrame.svelte';
  import OAuthButtons from './OAuthButtons.svelte';
  // Original shadcn-svelte login-02/signup-02 composition, shared with recovery.
  let {mode,busy=false,error='',notice='',returnTo='/discover',mfaChallenge='',mfaMethods=[],onSubmit,onPasskey,onMFA,onCancelMFA}: {
    mode:'login'|'register';busy?:boolean;error?:string;notice?:string;returnTo?:string;mfaChallenge?:string;mfaMethods?:string[];
    onSubmit:(fields:{email:string;password:string;name:string;handle:string})=>Promise<void>;
    onPasskey?:()=>Promise<void>;onMFA?:(code:string)=>Promise<void>;onCancelMFA?:()=>void;
  }=$props();
  let email=$state('');let password=$state('');let name=$state('');let handle=$state('');let code=$state('');
  const signup=$derived(mode==='register');
  const canCode=$derived(mfaMethods.includes('totp')||mfaMethods.includes('recovery'));
  const canPasskey=$derived(mfaMethods.includes('passkey'));
  $effect(()=>{if(mfaChallenge)password='';});
</script>
<AuthFrame>
  <form class="flex flex-col gap-6" onsubmit={async(event)=>{event.preventDefault();if(mfaChallenge&&onMFA){await onMFA(code.trim());code='';}else await onSubmit({email:email.trim(),password,name:name.trim(),handle:handle.trim()});}}>
    <div class="flex flex-col items-center gap-2 text-center"><h1 class="text-2xl font-bold">{mfaChallenge?'Verify your sign-in':signup?'Create an account':'Login to your account'}</h1><p class="text-sm text-balance text-muted-foreground">{mfaChallenge?(canCode?'Enter an authenticator code or an unused recovery code.':'Use a saved passkey to finish signing in.'):signup?'Create one account to watch, follow, and make videos.':'Enter your email below to login to your account.'}</p></div>
    {#if error}<div class="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive" role="alert">{error}</div>{/if}
    {#if notice}<div class="rounded-md border border-border bg-muted px-3 py-2 text-sm" role="status">{notice}</div>{/if}
    {#if !mfaChallenge}<OAuthButtons/>{/if}
    {#if page.url.searchParams.get('oauth_error')}<p class="cloud-alert" role="alert">{page.url.searchParams.get('oauth_error')==='account_exists'?'This email already has an account. Sign in with your existing method, then link this provider in Security settings.':'Provider sign-in could not finish. Try again, or use another sign-in method.'}</p>{/if}
    <div class="grid gap-6">
      {#if mfaChallenge}{#if canCode}<div class="grid gap-3"><Label for="mfa-code">Authenticator or recovery code</Label><Input id="mfa-code" bind:value={code} autocomplete="one-time-code" maxlength={100} required disabled={busy}/><p class="text-xs text-muted-foreground">Each code can be used once. If you just used an authenticator code, wait for the next one.</p></div><Button type="submit" disabled={busy||!code.trim()}>{#if busy}<LoaderCircle class="size-4 animate-spin"/>{/if}Verify and sign in</Button>{/if}{#if canPasskey&&onPasskey}<Button type="button" variant="outline" disabled={busy} onclick={onPasskey}><KeyRound class="size-4"/>Verify with a passkey</Button>{/if}<Button type="button" variant="outline" disabled={busy} onclick={()=>{code='';onCancelMFA?.();}}>Use another sign-in method</Button>
      {:else}
        {#if signup}<div class="grid gap-3"><Label for="signup-name">Display name</Label><Input id="signup-name" bind:value={name} autocomplete="name" placeholder="Your name" maxlength={100} required disabled={busy}/></div><div class="grid gap-3"><Label for="signup-handle">Username</Label><Input id="signup-handle" bind:value={handle} autocomplete="username" placeholder="your_username" pattern={'[a-zA-Z0-9_]{3,30}'} required disabled={busy}/><p class="text-xs text-muted-foreground">3–30 letters, numbers, or underscores. You can watch without setting up a creator page.</p></div>{/if}
        <div class="grid gap-3"><Label for={`${mode}-email`}>Email</Label><Input id={`${mode}-email`} type="email" bind:value={email} autocomplete="email" placeholder="m@example.com" required disabled={busy}/></div>
        <div class="grid gap-3"><div class="flex items-center"><Label for={`${mode}-password`}>Password</Label>{#if !signup}<a href={authHref('/forgot-password',returnTo,page.url.origin)} class="ml-auto text-sm underline-offset-4 hover:underline">Forgot your password?</a>{/if}</div><Input id={`${mode}-password`} type="password" bind:value={password} autocomplete={signup?'new-password':'current-password'} minlength={12} required disabled={busy}/>{#if signup}<p class="text-xs text-muted-foreground">Must be at least 12 characters long.</p>{/if}</div>
        <Button type="submit" class="w-full" disabled={busy}>{#if busy}<LoaderCircle class="size-4 animate-spin"/>{/if}{signup?'Create account':'Login'}</Button>
        {#if !signup}<a href={`/magic-link?returnTo=${encodeURIComponent(returnTo)}`} class="text-center text-sm underline underline-offset-4">Email me a sign-in link</a>{/if}{#if !signup&&onPasskey}<div class="relative text-center text-xs text-muted-foreground"><span>Or use your saved passkey</span></div><Button type="button" variant="outline" disabled={busy} onclick={onPasskey}><KeyRound class="size-4"/>Sign in with a passkey</Button>{/if}
      {/if}
    </div>
    {#if !mfaChallenge}<div class="text-center text-sm">{signup?'Already have an account?':"Don't have an account?"} <a href={authHref(signup?'/login':'/register',returnTo,page.url.origin)} class="underline underline-offset-4">{signup?'Sign in':'Sign up'}</a></div>{/if}
  </form>
</AuthFrame>
