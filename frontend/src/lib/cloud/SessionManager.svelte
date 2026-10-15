<script lang="ts">
 import {onMount} from 'svelte';
 import {Button} from '$lib/components/ui/button';
 import {LoaderCircle,Monitor} from '@lucide/svelte';
 import {request} from './api';
 interface Session {id:string;created_at:string;last_seen_at:string;expires_at:string;current:boolean}
 let sessions=$state<Session[]>([]),loading=$state(true),busy=$state(false),error=$state(''),notice=$state(''),confirmAll=$state(false);
 const date=(v:string)=>new Date(v).toLocaleString();
 async function load(){loading=true;error='';try{sessions=await request('/security/sessions');}catch(e){error=(e as Error).message;}finally{loading=false;}}
 onMount(()=>{void load();});
 async function revoke(id?:string){busy=true;error='';notice='';try{if(id)await request(`/security/sessions/${encodeURIComponent(id)}`,{},'DELETE');else await request('/security/sessions/revoke-others',{});confirmAll=false;await load();notice=id?'That session has been signed out.':'Other sessions have been signed out. This session stays active.';}catch(e){error=(e as Error).message;}finally{busy=false;}}
</script>
<section class="session-manager">
 <h2>Active sessions</h2><p class="session-help">Review where your account is signed in. Sessions are identified by their start and activity dates.</p>
 {#if error}<p class="cloud-alert" role="alert">{error}</p>{/if}{#if notice}<p class="cloud-notice" role="status">{notice}</p>{/if}
 {#if loading}<p class="session-help" role="status">Loading sessions…</p>{:else}
 <ul>{#each sessions as session}<li><Monitor class="size-4 shrink-0"/><div><strong>{session.current?'This session':'Another session'}</strong><p>Last active {date(session.last_seen_at)}</p><small>Started {date(session.created_at)} · Expires {date(session.expires_at)}</small></div>{#if session.current}<span class="current-session">Current</span>{:else}<Button variant="outline" size="sm" disabled={busy} onclick={()=>revoke(session.id)}>Sign out</Button>{/if}</li>{/each}</ul>
 <div class="session-actions">{#if confirmAll}<p class="session-help">Other browsers will need to sign in again.</p><Button variant="outline" disabled={busy} onclick={()=>revoke()}>{#if busy}<LoaderCircle class="size-4 animate-spin"/>{/if}Confirm sign out others</Button><Button variant="ghost" disabled={busy} onclick={()=>confirmAll=false}>Cancel</Button>{:else}<Button variant="outline" disabled={busy||sessions.filter(s=>!s.current).length===0} onclick={()=>confirmAll=true}>Sign out other sessions</Button><Button variant="ghost" disabled={busy} onclick={load}>Refresh sessions</Button>{/if}</div>
 {/if}
</section>
<style>
.session-manager{padding:30px 0;border-top:1px solid var(--border)}h2{font-size:18px;font-weight:600;margin:0 0 10px}.session-help,li p,li small{font-size:13px;color:var(--muted-foreground);line-height:1.7}.session-help{margin:0 0 18px}ul{padding:0;list-style:none}li{display:flex;align-items:center;gap:14px;padding:18px 0;border-bottom:1px solid var(--border)}li>div{flex:1;min-width:0}strong{font-size:13px}li p{margin:4px 0}li small{display:block;font-size:12px}.current-session{font-size:12px;color:var(--muted-foreground)}.session-actions{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-top:20px}.session-actions p{flex-basis:100%;margin:0}@media(max-width:650px){li{flex-wrap:wrap}li>div{flex-basis:calc(100% - 32px)}li>:global(button),.current-session{margin-left:30px}}
</style>
