<script lang="ts">
  import { Clapperboard, LoaderCircle } from '@lucide/svelte';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import { page } from '$app/state';
  import { authHref } from './auth';
  // Layout follows the official shadcn-svelte login-02 and signup-02 blocks.
  // https://shadcn-svelte.com/blocks/login
  // https://shadcn-svelte.com/blocks/sign-up
  let {mode,busy=false,error='',notice='',returnTo='/',onSubmit}: {
    mode:'login'|'register';busy?:boolean;error?:string;notice?:string;returnTo?:string;
    onSubmit:(fields:{email:string;password:string;name:string;handle:string})=>Promise<void>;
  }=$props();
  let email=$state('');let password=$state('');let name=$state('');let handle=$state('');
  const signup=$derived(mode==='register');
</script>
<div class="grid min-h-svh bg-background text-foreground lg:grid-cols-2">
  <div class="flex flex-col gap-4 p-6 md:p-10">
    <div class="flex justify-center gap-2 md:justify-start"><a href="/" class="flex items-center gap-2 font-medium"><span class="flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground"><Clapperboard class="size-4"/></span>VideoFlow</a></div>
    <div class="flex flex-1 items-center justify-center py-8"><div class="w-full max-w-xs">
      <form class="flex flex-col gap-6" onsubmit={async(event)=>{event.preventDefault();await onSubmit({email:email.trim(),password,name:name.trim(),handle:handle.trim()});}}>
        <div class="flex flex-col items-center gap-2 text-center"><h1 class="text-2xl font-bold">{signup?'Create an account':'Login to your account'}</h1><p class="text-sm text-balance text-muted-foreground">{signup?'Enter your information below to create your account.':'Enter your email below to login to your account.'}</p></div>
        {#if error}<div class="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive" role="alert">{error}</div>{/if}
        {#if notice}<div class="rounded-md border border-border bg-muted px-3 py-2 text-sm" role="status">{notice}</div>{/if}
        <div class="grid gap-6">
          {#if signup}<div class="grid gap-3"><Label for="signup-name">Name</Label><Input id="signup-name" bind:value={name} autocomplete="name" placeholder="Your name" maxlength={100} required disabled={busy}/></div><div class="grid gap-3"><Label for="signup-handle">Channel handle</Label><Input id="signup-handle" bind:value={handle} autocomplete="username" placeholder="your_channel" pattern={'[a-zA-Z0-9_]{3,30}'} required disabled={busy}/><p class="text-xs text-muted-foreground">3–30 letters, numbers, or underscores.</p></div>{/if}
          <div class="grid gap-3"><Label for={`${mode}-email`}>Email</Label><Input id={`${mode}-email`} type="email" bind:value={email} autocomplete="email" placeholder="m@example.com" required disabled={busy}/></div>
          <div class="grid gap-3"><div class="flex items-center"><Label for={`${mode}-password`}>Password</Label>{#if !signup}<a href={authHref('/forgot-password',returnTo,page.url.origin)} class="ml-auto text-sm underline-offset-4 hover:underline">Forgot your password?</a>{/if}</div><Input id={`${mode}-password`} type="password" bind:value={password} autocomplete={signup?'new-password':'current-password'} minlength={12} required disabled={busy}/>{#if signup}<p class="text-xs text-muted-foreground">Must be at least 12 characters long.</p>{/if}</div>
          <Button type="submit" class="w-full" disabled={busy}>{#if busy}<LoaderCircle class="size-4 animate-spin"/>{/if}{signup?'Create account':'Login'}</Button>
        </div>
        <div class="text-center text-sm">{signup?'Already have an account?':"Don't have an account?"} <a href={authHref(signup?'/login':'/register',returnTo,page.url.origin)} class="underline underline-offset-4">{signup?'Sign in':'Sign up'}</a></div>
      </form>
    </div></div>
  </div>
  <div class="relative hidden bg-muted lg:block" aria-hidden="true"></div>
</div>
