<script lang="ts">

 import {goto} from '$app/navigation';
 import {Button} from '$lib/components/ui/button';
 import {UserRound,ShieldCheck,MonitorPlay,Eye,Wallet,ArrowLeft,ArrowUpRight} from '@lucide/svelte';
 import ProfileEditor from '../ProfileEditor.svelte';
 import SecurityPanel from '../SecurityPanel.svelte';
 import Payments from '../Payments.svelte';
 import PlaybackSettings from './PlaybackSettings.svelte';
 import AccountSettings from './AccountSettings.svelte';
 import PrivacySettings from './PrivacySettings.svelte';
 let {user,section='account',invoiceId='',onUpdated}:{user:any;section:string;invoiceId?:string;onUpdated:(user:any)=>void}=$props();
 const sections=[{id:'account',name:'Account',icon:UserRound},{id:'profile',name:'Public profile',icon:UserRound},{id:'security',name:'Security',icon:ShieldCheck},{id:'playback',name:'Playback',icon:MonitorPlay},{id:'privacy',name:'Privacy',icon:Eye},{id:'billing',name:'Billing',icon:Wallet}];
</script>
<div class="settings-layout">
 <aside class="settings-nav"><a class="settings-back" href="/discover"><ArrowLeft class="size-4"/>Back to watching</a><h1>Settings</h1><nav aria-label="Settings sections">{#each sections as item}<a href={`/settings/${item.id}`} aria-current={section===item.id?'page':undefined}><item.icon class="size-4"/>{item.name}</a>{/each}</nav><label class="mobile-section"><span class="sr-only">Settings section</span><select aria-label="Settings section" value={section} onchange={e=>{const next=e.currentTarget.value;e.currentTarget.value=section;void goto(`/settings/${next}`);}}>{#each sections as item}<option value={item.id}>{item.name}</option>{/each}</select></label></aside>
 <div class="settings-content">
 {#if section==='account'}<AccountSettings {user}/>
 {:else if section==='profile'}<ProfileEditor onSaved={onUpdated}/>
 {:else if section==='security'}<SecurityPanel/>
 {:else if section==='playback'}<PlaybackSettings/>
 {:else if section==='privacy'}<PrivacySettings/>
 {:else if section==='billing'}<div class="page-heading"><h1>Billing</h1><p>Viewing access, generation credits, and payment history.</p></div><div class="billing-links"><a href="/premium"><strong>Premium viewing</strong><span>View your access and expiry <ArrowUpRight class="size-4"/></span></a><a href="/credits"><strong>Generation credits</strong><span>Balance and credit purchases <ArrowUpRight class="size-4"/></span></a></div><Payments {invoiceId}/>
 {:else}<div class="page-heading"><h1>Settings page not found</h1><p>Choose a section to manage your account.</p></div><Button href="/settings/account">Account settings</Button>{/if}
 </div>
</div>
<style>
.settings-nav .mobile-section{display:none!important}.settings-layout{display:grid;grid-template-columns:190px minmax(0,780px);gap:48px;max-width:1060px;margin:0 auto}.settings-nav{align-self:start;position:sticky;top:0}.settings-nav h1{font-size:20px;font-weight:600;margin:25px 0 20px}.settings-back{display:flex;gap:8px;align-items:center;font-size:12px;color:var(--muted-foreground)}nav{display:grid;gap:4px}nav a{display:flex;align-items:center;gap:12px;padding:11px 12px;border-radius:8px;font-size:13px;color:var(--muted-foreground)}nav a:hover,nav a[aria-current=page]{background:var(--accent);color:var(--foreground)}nav a[aria-current=page]{font-weight:600;color:var(--accent-foreground)}.settings-content{min-width:0}.settings-content :global(.security-page){max-width:none}.billing-links{display:grid;grid-template-columns:1fr 1fr;gap:20px;margin-bottom:32px}.billing-links a{padding:18px 0;border-block:1px solid var(--border)}.billing-links strong{display:block;font-size:14px}.billing-links span{display:flex;align-items:center;gap:8px;font-size:12px;color:var(--muted-foreground);margin-top:8px}@media(max-width:800px){.settings-layout{grid-template-columns:1fr;gap:28px}.settings-nav{position:static}.settings-nav h1{margin-block:18px 14px}nav{display:none}.settings-nav .mobile-section{display:block!important;margin:0!important}.mobile-section select{width:100%;min-height:44px;background:var(--background);color:var(--foreground);border:1px solid var(--border);border-radius:8px;padding:0 12px;font-size:14px}nav a{justify-content:center;gap:6px;padding:12px 6px;font-size:12px}nav a :global(svg){display:none}.billing-links{grid-template-columns:1fr;gap:0}}
</style>
