<script lang="ts">
  import { page } from '$app/state';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import { LoaderCircle, ArrowLeft } from '@lucide/svelte';
  import AuthFrame from './AuthFrame.svelte';
  import { authHref } from './auth';
  import { request } from './api';
  let {mode,returnTo='/discover',onVerified}:{mode:'forgot-password'|'reset-password'|'verify-email';returnTo?:string;onVerified?:()=>Promise<void>}=$props();
  let email=$state('');let token=$state('');let password=$state('');let confirmation=$state('');let busy=$state(false);let error=$state('');let notice=$state('');let developmentToken=$state('');let completed=$state(false);
  $effect(()=>{const incoming=page.url.searchParams.get('token');token=incoming||'';});
  $effect(()=>{mode;error='';notice='';completed=false;developmentToken='';password='';confirmation='';});
  const heading=$derived(mode==='forgot-password'?'Forgot your password?':mode==='reset-password'?'Set a new password':'Verify your email');
  async function submit(){
    busy=true;error='';notice='';
    try{
      if(mode==='reset-password'&&password!==confirmation)throw new Error('The passwords do not match. Enter the same new password in both fields.');
      const body=mode==='forgot-password'?{email:email.trim()}:mode==='verify-email'?{token:token.trim()}:{token:token.trim(),password};
      const endpoint=mode==='forgot-password'?'/auth/forgot-password':mode==='verify-email'?'/auth/verify-email':'/auth/reset-password';
      const result=await request(endpoint,body);developmentToken=result.development_token||'';completed=true;
      notice=mode==='forgot-password'?'If this address has an account, password reset instructions have been sent.':mode==='reset-password'?'Your password has been reset. Sign in with your new password.':'Your email is verified.';
      password='';confirmation='';if(mode==='verify-email')await onVerified?.();
    }catch(e){error=(e as Error).message;}finally{busy=false;}
  }
</script>
<AuthFrame>
  <form class="flex flex-col gap-6" onsubmit={(event)=>{event.preventDefault();void submit();}}>
    <div class="flex flex-col items-center gap-2 text-center"><h1 class="text-2xl font-bold">{heading}</h1><p class="text-sm text-balance text-muted-foreground">{mode==='forgot-password'?'Enter your email and we’ll help you get back to watching.':mode==='reset-password'?'Choose a password you haven’t used for this account.':'Enter the verification token from your email to confirm your address.'}</p></div>
    {#if error}<div role="alert" class="rounded-md border border-destructive/30 px-3 py-2 text-sm text-destructive">{error}</div>{/if}
    {#if notice}<div role="status" class="rounded-md border border-border bg-muted px-3 py-2 text-sm">{notice}</div>{/if}
    {#if !completed||mode==='forgot-password'}<div class="grid gap-6">
      {#if mode==='forgot-password'}<div class="grid gap-3"><Label for="recovery-email">Email</Label><Input id="recovery-email" type="email" bind:value={email} autocomplete="email" placeholder="m@example.com" required disabled={busy}/></div>{:else}<div class="grid gap-3"><Label for="recovery-token">Token from your email</Label><Input id="recovery-token" bind:value={token} autocomplete="one-time-code" required disabled={busy}/></div>{/if}
      {#if mode==='reset-password'}<div class="grid gap-3"><Label for="new-password">New password</Label><Input id="new-password" type="password" bind:value={password} minlength={12} required autocomplete="new-password" disabled={busy}/><p class="text-xs text-muted-foreground">At least 12 characters. Your two-step verification settings stay enabled.</p></div><div class="grid gap-3"><Label for="confirm-password">Confirm password</Label><Input id="confirm-password" type="password" bind:value={confirmation} minlength={12} required autocomplete="new-password" disabled={busy}/></div>{/if}
      <Button type="submit" disabled={busy}>{#if busy}<LoaderCircle class="size-4 animate-spin"/>{/if}{mode==='forgot-password'?(completed?'Send instructions again':'Send reset instructions'):mode==='reset-password'?'Reset password':'Verify email'}</Button>
    </div>{/if}
    {#if developmentToken}<div class="rounded-md border border-border p-3 text-xs"><p class="mb-2 font-medium">Local development only</p><a class="underline underline-offset-4" href={`/reset-password?token=${encodeURIComponent(developmentToken)}&returnTo=${encodeURIComponent(returnTo)}`}>Open the local password reset link</a></div>{/if}
    <a href={authHref('/login',returnTo,page.url.origin)} class="flex items-center justify-center gap-2 text-sm underline-offset-4 hover:underline"><ArrowLeft class="size-4"/>Back to sign in</a>
  </form>
</AuthFrame>
