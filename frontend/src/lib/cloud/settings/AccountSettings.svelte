<script lang="ts">
 import {Button} from '$lib/components/ui/button';
 import {request} from '../api';
 let {user}:{user:any}=$props();let busy=$state(false),error=$state(''),notice=$state(''),developmentToken=$state('');
 async function resend(){busy=true;error='';notice='';try{const result=await request('/auth/resend-verification',{email:user.email});developmentToken=result.development_token||'';notice='Check your inbox for verification instructions.';}catch(e){error=(e as Error).message;}finally{busy=false;}}
</script>
<div class="page-heading"><h1>Your account</h1><p>One account for watching and creating on 6eek.</p></div>
{#if error}<p class="cloud-alert" role="alert">{error}</p>{/if}{#if notice}<p class="cloud-notice" role="status">{notice}</p>{/if}
<section class="account-section"><h2>Personal details</h2><dl><div><dt>Display name</dt><dd>{user.name}</dd></div><div><dt>Email</dt><dd>{user.email}</dd></div><div><dt>Username</dt><dd>@{user.handle}</dd></div></dl><Button href="/settings/profile" variant="outline">Edit public profile</Button></section>
<section class="account-section"><h2>Email verification</h2><p>{user.verified?'Your email address is verified.':'Verify your email before publishing or using features that require a verified account.'}</p>{#if !user.verified}<div class="account-actions"><Button variant="outline" disabled={busy} onclick={resend}>{busy?'Sending…':'Resend verification email'}</Button><Button href="/verify-email?returnTo=%2Fsettings%2Faccount" variant="ghost">Enter verification code</Button></div>{#if developmentToken}<p class="field-help">Development-only code: <code>{developmentToken}</code></p>{/if}{/if}</section>
<style>.account-section{padding:26px 0;border-top:1px solid var(--border)}h2{font-size:16px;font-weight:600;margin:0 0 16px}dl{margin-bottom:22px}dl>div{display:grid;grid-template-columns:140px minmax(0,1fr);gap:20px;padding:14px 0;font-size:13px}dt,p{color:var(--muted-foreground)}dd{overflow-wrap:anywhere}p{font-size:13px;line-height:1.7}.account-actions{display:flex;gap:12px;flex-wrap:wrap;margin-top:20px}@media(max-width:600px){dl>div{grid-template-columns:1fr;gap:5px}.account-actions :global(button),.account-actions :global(a){min-height:44px}}
</style>
