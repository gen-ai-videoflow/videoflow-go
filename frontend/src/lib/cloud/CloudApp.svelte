<script lang="ts">
 import BrandLogo from '$lib/components/BrandLogo.svelte';
  import { restoredInvoiceAttempt } from './payment-state';
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Textarea } from '$lib/components/ui/textarea';
  import { Menu, UserRound, Compass, Clapperboard, FolderOpen, Coins, Radio, ChartNoAxesCombined, Settings, ArrowUpRight, ArrowRight, Plus, Play, Film, LogOut, LoaderCircle, Sparkles, Check, ShieldCheck, Heart, ListVideo, Users, Image, SlidersHorizontal, Crown, Wallet, LayoutTemplate } from '@lucide/svelte';
  import { request, CloudError } from './api';
  import { safeReturnTo, authHref } from './auth';
  import type { Project, Quote } from './generated';
  import './cloud.css';
  import ViewerShell from './ViewerShell.svelte';
 import CreatorShell from './CreatorShell.svelte';
 import ContentManager from './publishing/ContentManager.svelte';
  import SettingsPage from './settings/SettingsPage.svelte';
  import OutputDetail from './publishing/OutputDetail.svelte';
  import PublishComposer from './publishing/PublishComposer.svelte';
  import UploadVideo from './publishing/UploadVideo.svelte';
  import DiscoverFeed from './viewing/DiscoverFeed.svelte';
  import WatchPage from './viewing/WatchPage.svelte';
  import ShortsFeed from './viewing/ShortsFeed.svelte';
  import {settingsSection,studioSection} from './navigation';
 import Dashboard from './Dashboard.svelte';
 import EditorPage from './EditorPage.svelte';
 import * as Dropdown from '$lib/components/ui/dropdown-menu';
  import PublicProfile from './PublicProfile.svelte';
  import Studio from './Studio.svelte';
 import PromptWorkbench from './prompts/PromptWorkbench.svelte';
 import {promptBytes} from './prompts/render.mjs';
  import OriginalStudio from './OriginalStudio.svelte';
  import AuthForm from './AuthForm.svelte';
  import RecoveryForm from './RecoveryForm.svelte';
  import { usePasskey } from './webauthn';
  import Payments from './Payments.svelte';
  import Templates from './Templates.svelte';
  import GenerationJobs from './GenerationJobs.svelte';
  let sidebarCollapsed = $state(false);
  function toggleSidebar(){sidebarCollapsed=!sidebarCollapsed;try{localStorage.setItem("videoflow.studio.sidebar",sidebarCollapsed?"collapsed":"expanded");}catch{}}
  let user = $state<any>(null);
  let sessionKnown=$state(false);
  let ready = $state(!!page.data.cloudPublic);
  let loading = $state(!page.data.cloudPublic);
  let loadFailed = $state(false);
  let busy = $state(false);
  let error = $state('');
  let notice = $state('');
  let data = $state<any>(page.data.cloudPublic || {});
  let projects = $state<Project[]>([]);
  let jobs = $state<any[]>([]);
  let mfaChallenge=$state('');let mfaMethods=$state<string[]>([]);
  let prompt = $state(''); let duration = $state('10'); let ratio = $state('16:9'); let projectId = $state('');
  let projectName = $state(''); let projectDescription = $state('');
  let quote = $state<Quote | null>(null); let quoteKey = '';
  let channelName = $state(''); let channelDescription = $state('');
  let title = $state(''); let description = $state(''); let jobId = $state(''); let visibility = $state('private'); let audience = $state('free');
  let playback = $state('');
  let payout = $state<any>({}); let payoutCountry = $state('');
  let reportReason = $state(''); let showReport = $state(false); let followed = $state(false);
  const invoiceAttempts=new Map<string,{key:string;id?:string}>();
  function createInvoice(purpose:'credits'|'premium',packId?:string){
    if(!user){void goto(authHref('/login','/premium',page.url.origin));return;}
    void action(async()=>{
      const storageKey=`videoflow.payment.${user.id}.${purpose}.${packId || ''}`;
      let attempt=invoiceAttempts.get(storageKey);
      if(!attempt){try{const saved=JSON.parse(sessionStorage.getItem(storageKey)||'null');attempt=restoredInvoiceAttempt(saved);}catch{}}
      if(attempt?.id){const existing=await request(`/payments/invoices/${encodeURIComponent(attempt.id)}`);if(existing.status!=='settled'){await goto(`/payments?invoice=${encodeURIComponent(existing.id)}`);return;}attempt=undefined;}
      if(!attempt)attempt={key:crypto.randomUUID()};
      invoiceAttempts.set(storageKey,attempt);try{sessionStorage.setItem(storageKey,JSON.stringify(attempt));}catch{}
      const invoice=await request('/payments/invoices',{purpose,...(packId?{pack_id:packId}:{})},'POST',{'Idempotency-Key':attempt.key});
      if(!invoice.id)throw new Error('The invoice identifier was not returned. Check Payments before creating another invoice.');
      attempt.id=invoice.id;try{sessionStorage.setItem(storageKey,JSON.stringify(attempt));}catch{}
      await goto(`/payments?invoice=${encodeURIComponent(invoice.id)}`);
    });
  }
  let developmentToken = $state('');
  const rawPath=$derived(page.url.pathname);
  const settingsTab=$derived(settingsSection(rawPath));
  const aliases:Record<string,string>={'/studio/projects':'/projects','/studio/templates':'/templates','/studio/videos':'/render','/studio/channel':'/channel','/studio/earnings':'/earnings','/studio/content':'/channel','/studio/scenes':'/scenes','/studio/characters':'/characters','/studio/assets':'/assets'};
  const path = $derived(aliases[rawPath] || rawPath);
  let searchQuery=$state('');
  $effect(()=>{searchQuery=page.url.searchParams.get('q') || '';});
  const feedQuery=$derived(page.url.searchParams.get('q') || '');
  const recoveryPage = $derived(['/forgot-password','/reset-password','/verify-email'].includes(path));
  const authPage = $derived(path === '/login' || path === '/register' || recoveryPage);
  const signInPage=$derived(path==='/login'||path==='/register');
  const resourceSection=$derived(studioSection(path,page.url.searchParams.get('tab')||''));
  const canvasPage=$derived(path==='/studio'&&!resourceSection);
  const dashboardPage=$derived(path==='/studio/dashboard');
  const editorPage=$derived(path==='/studio/editor');
  const viewingRoute=$derived(['/','/discover','/following','/search','/library','/history','/shorts'].includes(path)||path.startsWith('/watch/')||path.startsWith('/shorts/'));
  const creatorUtilityPage=$derived(path.startsWith('/studio/publish')||path.startsWith('/studio/upload')||path.startsWith('/studio/outputs/')||path==='/channel');
  const viewerPage=$derived(viewingRoute||!!settingsTab||['/', '/discover','/following','/search','/settings','/security','/profile','/payments','/premium','/memberships','/credits'].includes(path)||path.startsWith('/watch/')||path.startsWith('/channels/'));
  const authReturnTo=$derived(safeReturnTo(page.url.searchParams.get('returnTo') || ((canvasPage||resourceSection||dashboardPage||editorPage) ? page.url.pathname+page.url.search+page.url.hash : '/discover'),page.url.origin));
  const signInHref=$derived(authHref('/login',authPage ? authReturnTo : page.url.pathname+page.url.search+page.url.hash,page.url.origin));
  const signUpHref=$derived(authHref('/register',authPage ? authReturnTo : page.url.pathname+page.url.search+page.url.hash,page.url.origin));
  const displayAuth=$derived(signInPage||((canvasPage||resourceSection||dashboardPage||editorPage)&&sessionKnown&&!user));
  const publicPage = $derived(path==='/shorts'||path.startsWith('/shorts/')||['/','/discover','/search'].includes(path)||path.startsWith('/watch/')||path.startsWith('/channels/')||path==='/premium'||path==='/memberships');
  const createNav=[{url:'/create',label:'Create video',icon:Clapperboard},{url:'/studio/templates',label:'Templates',icon:LayoutTemplate}];
  const browseNav=[{url:'/discover',label:'Discover',icon:Compass}];
  const outputNav=[{url:'/studio/videos',label:'My videos',icon:Film}];
  const advancedNav=[{url:'/studio/editor',label:'Editor',icon:Clapperboard},{url:'/studio',label:'Storyboard',icon:Clapperboard},{url:'/workspace',label:'Project workspace',icon:FolderOpen}];
  const systemNav=[{url:'/channel',label:'My content',icon:Radio}];
  const mobileNav=[{url:'/studio/dashboard',label:'Overview',icon:LayoutTemplate},{url:'/projects',label:'Projects',icon:FolderOpen},...createNav,...outputNav,...advancedNav,...browseNav,...systemNav];
  const active=(url:string)=>url==='/workspace'?!!resourceSection:url==='/studio'?canvasPage:path===(aliases[url]||url);
  const titles: Record<string,string> = {'/':'Discover','/shorts':'Shorts','/library':'Saved videos','/history':'Watch history','/following':'Following','/search':'Search','/profile':'Edit profile','/security':'Security','/payments':'Payments','/templates':'Templates','/discover':'Discover','/create':'Create video','/projects':'Projects','/credits':'Credits & billing','/channel':'Your channel','/earnings':'Creator earnings','/studio':'Canvas','/studio/dashboard':'Projects overview','/studio/editor':'Video editor','/workspace':'Project workspace','/settings':'Account settings','/memberships':'Premium','/premium':'Premium','/render':'My videos','/scenes':'Scenes','/characters':'Characters','/assets':'Assets'};
  const list = (value:any):any[] => Array.isArray(value) ? value : value?.items || value?.videos || value?.projects || value?.jobs || [];
  const money = (cents:number | undefined, currency = 'USD') => cents == null ? '—' : new Intl.NumberFormat('en-US',{style:'currency',currency}).format(cents/100);
  const date = (value:string) => value ? new Date(value).toLocaleDateString(undefined,{month:'short',day:'numeric',year:'numeric'}) : '—';
  onMount(() => {
    try{sidebarCollapsed=localStorage.getItem("videoflow.studio.sidebar")==="collapsed";}catch{}
    request('/me').then(v => user = v.user || v).catch(e => {
      if (!(e instanceof CloudError && e.status === 401)) error = e.message;
    }).finally(() => {ready=true;sessionKnown=true;});

  });
  $effect(() => { if (ready) { const current = path; const query=feedQuery; if (!user && !query && page.data.cloudPath === current && page.data.cloudPublic) { data = page.data.cloudPublic; loading = false; } else void load(current); } });
  let loadSequence=0;
  async function load(current:string) {
    const sequence=++loadSequence;
    loading = true; loadFailed = false; error = ''; notice = ''; data = {}; playback = ''; quote = null; showReport = false; reportReason = ''; followed = false;
    if (authPage||settingsTab||viewingRoute||!!resourceSection||dashboardPage||editorPage||current==='/channel'||rawPath.startsWith('/studio/outputs/')||rawPath==='/studio/publish'||rawPath==='/studio/upload') { loading = false; return; }
    if (!user && !publicPage) { loading = false; return; }
    try {
      let candidate:any={};let candidateJobs:any[]|undefined;
      if (canvasPage) candidate = {};
      else if(['/', '/discover','/following','/search'].includes(current))candidate={videos:list(await request(`/feed?${new URLSearchParams({...((current==='/following')?{filter:'following'}:{}),...(feedQuery?{q:feedQuery}:{})})}`))};
      else if (current === '/projects') candidate = {projects:list(await request('/projects'))};
      else if (current === '/create') { const [p,j] = await Promise.all([request('/projects'),request('/jobs')]); projects = list(p); jobs = list(j); const requestedProject=page.url.searchParams.get('project'); if(requestedProject&&projects.some((p:any)=>p.id===requestedProject)) projectId=requestedProject; }
      else if (current === '/credits') { const [balance,payments] = await Promise.all([request('/credits'),request('/payments/options')]); candidate = {...balance,payments}; }
      else if (current === '/channel') { const [channel,generations] = await Promise.all([request('/channel'),request('/jobs')]);candidate=channel; jobs=list(generations); channelName = channel.channel?.name || channel.name || ''; channelDescription = channel.channel?.description || channel.description || ''; }
      else if (current === '/earnings') { const [earnings,analytics,account] = await Promise.all([request('/creator/earnings'), request('/creator/analytics'),request('/creator/payout-account')]); candidate = {...earnings,analytics}; payout=account;payoutCountry=account.country || ''; }
      else if (current === '/premium'||current === '/memberships'){const [premium,payments]=await Promise.all([request('/premium'),request('/payments/options')]);candidate={...premium,payments};}
      else if(current==='/render')candidateJobs=list(await request('/jobs'));
      else if (current.startsWith('/watch/')) { candidate = await request(`/videos/${encodeURIComponent(current.split('/')[2])}`); }
      else if (current.startsWith('/channels/')) candidate = await request(`/channels/${encodeURIComponent(current.split('/')[2])}`);
      if(sequence===loadSequence){data=candidate;if(candidateJobs)jobs=candidateJobs;}
    } catch(e) { if(sequence===loadSequence){error = (e as Error).message; loadFailed = true;} } finally { if(sequence===loadSequence)loading = false; }
  }
  async function action(fn:()=>Promise<void>) { busy = true; error = ''; notice = ''; try { await fn(); } catch(e) { error = (e as Error).message; } finally { busy = false; } }
  async function authenticate(fields:{email:string;password:string;name:string;handle:string}) {
    const target=authReturnTo;const signup=path==='/register';
    await action(async()=>{const result=await request(signup?'/auth/register':'/auth/login',signup?fields:{email:fields.email,password:fields.password});if(result.mfa_required){mfaChallenge=result.challenge;mfaMethods=result.methods||[];return;}mfaChallenge='';mfaMethods=[];user=await request('/me');await goto(target);});
  }
  async function completeMFA(code:string){const target=authReturnTo;await action(async()=>{await request('/auth/mfa',{challenge:mfaChallenge,code});mfaChallenge='';mfaMethods=[];user=await request('/me');await goto(target);});}
  async function signInWithPasskey(){const target=authReturnTo;await action(async()=>{const begin=await request('/auth/passkeys/options',{});const credential=await usePasskey(begin.options);await request('/auth/passkeys/verify',{challenge:begin.challenge,credential});mfaChallenge='';mfaMethods=[];user=await request('/me');await goto(target);});}

  function getQuote() { void action(async () => { quote = await request('/generation/quotes',{prompt,duration:Number(duration),aspect_ratio:ratio,project_id:projectId || undefined}); quoteKey = crypto.randomUUID(); }); }
  function generate() { const quoteId=quote?.id; if(!quoteId)return; void action(async () => { await request('/generation/jobs',{quote_id:quoteId},'POST',{'Idempotency-Key':quoteKey}); quote = null; jobs = list(await request('/jobs')); notice = 'Generation submitted. Your progress is saved here.'; }); }
  function createProject() { void action(async () => { await request('/projects',{name:projectName,description:projectDescription}); projectName = ''; projectDescription = ''; data = {projects:list(await request('/projects'))}; notice = 'Project created.'; }); }
  function publish() { void action(async () => { const video = await request('/videos',{job_id:jobId,title,description,visibility,audience}); await request(`/videos/${video.id}/publish`,{}); notice = visibility === 'private' ? 'Private video saved and submitted for processing.' : 'Video submitted for publishing.'; data = await request('/channel'); }); }
</script>

<svelte:head><title>{(settingsTab?`Settings — ${settingsTab}`:rawPath.startsWith('/studio/outputs/')?'Your video':rawPath==='/studio/publish'?'Publish video':rawPath==='/studio/upload'?'Upload video':path.startsWith('/shorts')?'Shorts':titles[path]) || (authPage ? 'Welcome' : data.title || data.channel?.name || data.name || 'Watch')} · VideoFlow</title>{#if publicPage}<link rel="canonical" href={page.url.origin + path}/><meta property="og:title" content={`${data.title || data.channel?.name || data.name || titles[path] || 'VideoFlow'} · VideoFlow`}/><meta property="og:description" content={data.description || data.channel?.description || 'VideoFlow video studio.'}/>{/if}<meta name="description" content="Create your next story with VideoFlow. Your video studio, channel, and creative community." /></svelte:head>
{#if recoveryPage}
  <RecoveryForm mode={path==='/forgot-password'?'forgot-password':path==='/reset-password'?'reset-password':'verify-email'} returnTo={authReturnTo} onVerified={async()=>{try{user=await request('/me');}catch{user=null;}}}/>
{:else if canvasPage&&!sessionKnown}
  <div class="flex h-screen items-center justify-center gap-2 bg-background text-sm text-muted-foreground" role="status"><LoaderCircle class="size-4 animate-spin"/>Loading VideoFlow…</div>
{:else if displayAuth}
  <AuthForm mode={path==='/register'?'register':'login'} {busy} {error} {notice} returnTo={authReturnTo} onSubmit={authenticate} {mfaChallenge} {mfaMethods} onMFA={completeMFA} onPasskey={signInWithPasskey} onCancelMFA={()=>{mfaChallenge='';mfaMethods=[];error='';}}/>
{:else}
{#snippet cloudContent()}
  <div class="cloud-body min-h-0 min-w-0 flex-1 overflow-hidden">
    {#if !canvasPage&&!editorPage&&!resourceSection&&!dashboardPage&&!viewerPage&&!creatorUtilityPage}<header class="cloud-topbar"><span>{titles[path] || 'VideoFlow'}</span><div class="flex items-center gap-4"><a href="/premium" class="inline-flex items-center gap-1.5 text-xs text-muted-foreground"><Crown class="size-3.5"/>Premium</a>{#if !user}<Button href={signInHref} variant="outline" size="sm">Sign in</Button>{/if}</div></header>{/if}
    <main class="cloud-main" class:cloud-canvas-main={canvasPage||editorPage} class:viewer-main={viewerPage}>
      {#if error}<div class="cloud-alert" role="alert"><span>{error}</span><button onclick={() => load(path)}>Try again</button></div>{/if}
      {#if notice}<div class="cloud-notice" role="status"><Check size={17}/>{notice}</div>{/if}
      {#if !ready || loading || (viewingRoute&&!sessionKnown)}<div class="cloud-loading" role="status"><LoaderCircle class="animate-spin" size={25}/><span>{settingsTab?'Loading settings…':path.startsWith('/studio')?'Loading Studio…':'Loading videos…'}</span></div>
      {:else if loadFailed}<div class="cloud-empty"><Film size={35}/><h1>Let's try that again.</h1><p>This page couldn't be loaded. Your saved work is still in your account.</p><Button onclick={()=>load(path)}>Reload page</Button></div>
      {:else if !user && !publicPage}
        <div class="cloud-empty"><ShieldCheck size={38}/><h1>{viewerPage?'Sign in to continue':'Your studio is waiting.'}</h1><p>{viewerPage?'Use your VideoFlow account to follow creators and manage your profile, payments, and preferences.':'Sign in to create videos, manage projects, and grow your channel.'}</p><Button href={signInHref}>Sign in <ArrowRight size={16}/></Button><a href={signUpHref}>Create an account</a></div>
      {:else if rawPath.startsWith('/studio/outputs/')}
        {#key rawPath}<OutputDetail jobId={rawPath.split('/')[3]}/>{/key}
      {:else if rawPath==='/studio/upload'}
        <UploadVideo {user}/>
      {:else if rawPath==='/studio/publish'}
        {#key page.url.search}<PublishComposer/>{/key}
      {:else if settingsTab}
        <SettingsPage {user} section={settingsTab} invoiceId={page.url.searchParams.get('invoice')||''} onUpdated={(updated)=>{if(updated)user=updated;}}/>
      {:else if editorPage}
        <EditorPage {user}/>
      {:else if dashboardPage}
        <Dashboard {user}/>
      {:else if resourceSection}
        {#key path+page.url.search}<Studio initialSection={resourceSection}/>{/key}
      {:else if canvasPage}
        <OriginalStudio/>
      {:else if ['/','/discover','/following','/search','/library','/history'].includes(path)}
        {#key path+page.url.search}<DiscoverFeed {user} following={path==='/following'} library={path==='/library'?'saved':path==='/history'?'history':''}/>{/key}
      {:else if path==='/shorts'||path.startsWith('/shorts/')}
        <ShortsFeed {user} initialId={path.split('/')[2]||''}/>
      {:else if path === '/create'}
        <div class="page-heading"><h1>Create video</h1><p>Describe your video. Review the cost. Bring it to life.</p></div><div class="story-entry"><div><h2>Have a whole story in mind?</h2><p>Plan a screenplay, generate its scenes, and assemble a complete video in your project storyboard.</p></div><Button variant="outline" href={`/studio?tab=storyboard${projectId ? '&project='+encodeURIComponent(projectId) : ''}`}>Open story-to-video <ArrowUpRight size={16}/></Button></div><div class="section-heading clip-heading"><h2>Or start with a single clip</h2></div><div class="creation-layout"><form class="cloud-panel create-form" onsubmit={(e)=>{e.preventDefault();getQuote();}}>{#if projectId}{#key projectId}<div class="mb-5"><PromptWorkbench {projectId} category="video" onapply={(value)=>{prompt=value;quote=null;}}/></div>{/key}{:else}<p class="field-help">Choose a project below to use saved bilingual prompt templates.</p>{/if}<label for="prompt" class="strong-label">The idea</label><Textarea id="prompt" bind:value={prompt} oninput={()=>quote=null} placeholder="A quiet coastal town at sunrise. A cyclist follows the winding road as the first light reaches the sea…" required minlength={10} maxlength={8000} rows={7}/><p class="field-help">Set the scene, describe the action, and tell us how it should feel.</p><div class="form-grid"><div><label for="duration">Duration</label><select id="duration" bind:value={duration} onchange={()=>quote=null}><option value="5">5 seconds</option><option value="10">10 seconds</option><option value="15">15 seconds</option><option value="30">30 seconds</option></select></div><div><label for="ratio">Frame</label><select id="ratio" bind:value={ratio} onchange={()=>quote=null}><option>16:9</option><option>9:16</option><option>1:1</option></select></div></div><label for="project">Project</label><select id="project" bind:value={projectId} onchange={()=>quote=null}><option value="">No project</option>{#each projects as project}<option value={project.id}>{project.name}</option>{/each}</select><div class="form-footer"><span><ShieldCheck size={15}/>Private until you publish</span><Button type="submit" disabled={busy || prompt.length < 10 || promptBytes(prompt)>8000}>Review credit cost <ArrowRight size={15}/></Button></div>{#if quote}<div class="quote-box"><div><strong>{quote.credits} credits</strong><p>Quote expires {new Date(quote.expires_at).toLocaleTimeString()}. Credits are reserved when you confirm.</p></div><Button type="button" disabled={busy || Date.parse(quote.expires_at) < Date.now()} onclick={generate}>Confirm generation</Button></div>{/if}</form><aside class="creative-guide"><h2>Give your idea<br/>a little direction.</h2><ol><li><span>01</span><div><strong>Build a world</strong><p>Where are we? A place, a time of day, a feeling.</p></div></li><li><span>02</span><div><strong>Find the movement</strong><p>Describe what happens, from subject to camera.</p></div></li><li><span>03</span><div><strong>Make it yours</strong><p>Natural light, a cinematic mood, or something unexpected.</p></div></li></ol><div class="guide-foot"><Coins size={17}/><p>You see the credit cost before any generation starts.</p></div></aside></div><GenerationJobs {jobs} onrefresh={async()=>{jobs=list(await request('/jobs'));}}/>
      {:else if path === '/workspace'}
        <Studio/>
      {:else if path === '/render'}
        <div class="page-heading"><h1>My videos</h1><p>Generation progress and completed outputs.</p></div><GenerationJobs {jobs} onrefresh={async()=>{jobs=list(await request('/jobs'));}}/>
      {:else if path==='/payments'}
        <Payments invoiceId={page.url.searchParams.get('invoice') || ''}/>
      {:else if path==='/templates'}
        <Templates userId={user.id}/>
      {:else if path === '/projects'}
        <div class="page-heading"><span class="eyebrow">YOUR WORK, TOGETHER</span><h1>Projects</h1><p>Keep your creative work organized in private projects.</p></div><div class="projects-layout"><section>{#if data.projects?.length}<div class="project-grid">{#each data.projects as project}<article class="cloud-panel project-card"><FolderOpen size={26}/><h2>{project.name}</h2><p>{project.description || 'A fresh space for your next video.'}</p><small>Created {date(project.created_at)}</small><Button variant="outline" href={`/studio?project=${project.id}`}>Open studio <ArrowUpRight size={14}/></Button></article>{/each}</div>{:else}<div class="cloud-empty compact"><FolderOpen size={35}/><h2>Big ideas need a little space.</h2><p>Create your first project to keep your work together.</p></div>{/if}</section><form class="cloud-panel" onsubmit={(e)=>{e.preventDefault();createProject();}}><h2>New project</h2><label for="project-name">Project name</label><Input id="project-name" bind:value={projectName} placeholder="My next great idea" required maxlength={100}/><label for="project-description">Description <span class="subtle">(optional)</span></label><Textarea id="project-description" bind:value={projectDescription} rows={4}/><Button type="submit" disabled={busy || !projectName.trim()}><Plus size={16}/>Create project</Button></form></div>
      {:else if path === '/credits'}
        <div class="page-heading"><span class="eyebrow">FUEL YOUR NEXT IDEA</span><h1>Credits & billing</h1><p>Credits power your generations. Your confirmed balance is always here.</p></div>{#if data.restricted || data.debt > 0}<div class="cloud-alert" role="alert"><div><strong>{data.restricted ? 'Generation is restricted on your account.' : 'Your account has an outstanding credit balance.'}</strong>{#if data.debt > 0}<p>Outstanding debt: {data.debt.toLocaleString()} credits.</p>{/if}<p>Review your credit activity for payment reversals and adjustments.</p></div></div>{/if}<div class="balance-strip"><div><span>Available credits</span><strong>{data.available ?? '—'}<Coins size={25}/></strong></div><div><span>Reserved for generation</span><strong>{data.reserved ?? '—'}</strong></div><p>Generation credits are used in the studio and cannot be withdrawn as cash.</p></div><div class="section-heading"><h2>Top up your creativity</h2></div>{#if !data.payments?.enabled}<div class="info-banner"><Coins size={20}/><div><strong>Crypto payments are not available yet.</strong><p>Credit packs become available when the payment network, token, and invoice service are configured.</p></div></div>{/if}<div class="pack-grid">{#each data.payments?.credit_packs || [] as pack}<article class="cloud-panel"><span class="eyebrow">{pack.name || pack.id}</span><h2>{pack.credits.toLocaleString()} <small>credits</small></h2><p class="pack-price">{money(pack.price_cents,pack.currency || 'USD')}</p><Button disabled={!data.payments?.enabled || busy} onclick={()=>createInvoice('credits',pack.id)}>Get payment invoice <ArrowUpRight size={15}/></Button></article>{/each}</div><p class="field-help"><a href="/payments" class="underline underline-offset-4">Recover or check an existing payment invoice</a>. Credits are added only after the server confirms settlement.</p><section class="jobs-section"><h2>Credit activity</h2>{#if data.transactions?.length}<div class="record-list">{#each data.transactions as transaction}<div class="record"><Coins size={18}/><div><strong>{transaction.description || transaction.kind}</strong><small>{date(transaction.created_at)}</small></div><strong>{transaction.amount ?? transaction.credits}</strong></div>{/each}</div>{:else}<p class="quiet-empty">Purchases, reservations, and refunds will appear here.</p>{/if}</section>
      {:else if path === '/channel'}
        <ContentManager {user}/>
      {:else if path === '/earnings'}
        <div class="page-heading"><span class="eyebrow">CREATOR BUSINESS</span><h1>Creator earnings</h1><p>Track your audience and confirmed creator earnings.</p></div><div class="metric-grid">{#each [{label:'Validated views',value:data.analytics?.views},{label:'Watch minutes',value:data.analytics?.watch_minutes},{label:'Followers',value:data.analytics?.followers},{label:'Published videos',value:data.analytics?.videos}] as metric}<div><span>{metric.label}</span><strong>{metric.value ?? '—'}</strong></div>{/each}</div><div class="section-heading"><h2>Earnings</h2><span class="subtle">{data.currency || 'USD'}</span></div><div class="balance-strip"><div><span>Available</span><strong>{money(data.available_cents,data.currency)}</strong></div><div><span>Pending</span><strong>{money(data.pending_cents,data.currency)}</strong></div><div><span>Paid to date</span><strong>{money(data.paid_cents,data.currency)}</strong></div></div>{#if !data.rewards_enabled}<div class="info-banner"><ShieldCheck size={21}/><div><strong>Creator rewards are not enabled.</strong><p>Eligibility, rewards policy, and payouts must be approved before earnings can be activated. There is no guaranteed payment per view.</p></div></div>{/if}<section class="cloud-panel payout-panel"><div><span class="eyebrow">GET READY TO EARN</span><h2>Payout account</h2><p>{payout.configured ? `Account connected${payout.country ? ' in ' + payout.country : ''}.` : 'Connect an eligible account to complete identity and payment verification.'}</p><p class="field-help">{payout.payouts_enabled ? 'Payment provider account verified.' : 'Payment provider verification is incomplete.'} {payout.cash_payouts_enabled ? 'Cash payouts are enabled.' : 'Cash payouts are not enabled. Connecting an account does not activate payouts.'}</p></div><form onsubmit={(e)=>{e.preventDefault();action(async()=>{const result=await request('/creator/payout-account',{country:payoutCountry.toUpperCase()});window.location.assign(result.url);});}}><label for="payout-country">Country code</label><Input id="payout-country" bind:value={payoutCountry} placeholder="e.g. US" pattern="[A-Za-z][A-Za-z]" maxlength={2} required disabled={payout.configured}/><p class="field-help">Use your legal residence's two-letter country code. Availability is checked before onboarding.</p><Button type="submit" disabled={busy || !payout.onboarding_enabled || !user.verified}>{payout.configured ? 'Continue verification' : 'Set up payout account'}<ArrowUpRight size={15}/></Button>{#if !payout.onboarding_enabled}<p class="field-help">Payout onboarding is not available yet.</p>{:else if !user.verified}<p class="field-help">Verify your email in account settings to continue.</p>{/if}</form></section><h2 class="jobs-section">Earnings activity</h2>{#each data.entries || [] as entry}<div class="record"><div><strong>{entry.description || entry.kind}</strong><small>{date(entry.created_at)}</small></div><span>{money(entry.amount_cents,data.currency)}</span></div>{:else}<p class="quiet-empty">Confirmed earnings and adjustments will appear here.</p>{/each}
      {:else if path === '/premium'||path === '/memberships'}
        <div class="page-heading"><h1>VideoFlow Premium</h1><p>Prepaid viewing access across VideoFlow. The same account can watch, create, and publish. Generation credits are separate.</p></div>
        <section class="cloud-panel premium-panel"><Crown class="size-7 text-muted-foreground"/><h2>{data.active?'Your Premium access':'Get 30 days of Premium'}</h2>{#if data.active}<span class="status">{data.status || 'Active'}</span><p>Premium access is active{data.paid_through ? ` through ${date(data.paid_through)}` : ''}.</p>{:else}<p>Watch Premium videos from creators across the platform with one account.</p>{/if}{#if data.payments?.premium_price_cents>0}<strong class="premium-price">{money(data.payments.premium_price_cents,'USD')}<span>for 30 days</span></strong>{/if}<p>Crypto Premium is prepaid for 30 days and does not renew automatically. An additional confirmed payment extends access from the later of your current expiry or today.</p>{#if !data.payments?.enabled||!data.payments?.premium_price_cents}<div class="info-banner"><ShieldCheck class="size-5"/><p>New Premium payments are not available yet. Existing access is shown above; payment availability depends on the configured token and network.</p></div>{/if}<Button disabled={busy||!data.payments?.enabled||!data.payments?.premium_price_cents} onclick={()=>createInvoice('premium')}>{user?(data.active?'Add 30 days':'Get 30 days'):'Sign in to get Premium'}<ArrowUpRight class="size-4"/></Button>{#if user&&data.can_manage}<Button variant="outline" disabled={busy} onclick={()=>action(async()=>{const result=await request('/premium/manage',{});window.location.assign(result.url);})}>Manage historical subscription <ArrowUpRight class="size-4"/></Button>{/if}<p class="field-help">New payments use {data.payments?.token || 'the configured token'} on {data.payments?.network || 'the configured network'}. Your invoice shows the exact token amount, fees/reserve, and deposit address. Access is granted only after server-confirmed settlement.</p><a href="/payments" class="text-sm underline underline-offset-4">View payment invoices</a><p class="field-help">Premium grants viewing access. It does not include studio generation credits or guarantee creator earnings.</p></section>
      {:else if path.startsWith('/watch/')}
        {#key path}<WatchPage {user} id={path.split('/')[2]}/>{/key}
      {:else if path.startsWith('/channels/')}
        <PublicProfile {data} {user} {busy} followed={followed || data.is_following===true} onFollow={()=>{if(!user){void goto(signInHref);return;}void action(async()=>{const wasFollowing=followed||data.is_following===true;await request(`/channels/${data.id || data.channel?.id}/follow`,{},wasFollowing?'DELETE':'POST');followed=!wasFollowing;data=await request(`/channels/${encodeURIComponent(path.split('/')[2])}`);notice=followed?'You are now following this creator.':'You unfollowed this creator.';});}}/>
      {:else}<div class="cloud-empty"><Compass size={35}/><h1>A little off the beaten path.</h1><p>This page isn't part of your cloud studio.</p><Button href="/studio">Back to studio</Button></div>{/if}

    </main>
  </div>
{/snippet}
{#if viewerPage}<ViewerShell {user} {path} {signInHref} onSignOut={()=>action(async()=>{await request('/auth/logout',{});user=null;await goto('/');})}>{@render cloudContent()}</ViewerShell>{:else if creatorUtilityPage}<CreatorShell {path}>{@render cloudContent()}</CreatorShell>{:else if editorPage}<div class="cloud-app flex h-screen min-w-0 flex-col bg-background text-foreground">{@render cloudContent()}</div>{:else}<div class="cloud-app flex h-screen min-w-0 flex-col bg-background text-foreground md:flex-row">
  <nav aria-label="Primary" class="studio-mobile-header md:hidden"><a href="/studio/dashboard"><BrandLogo compact/></a><Dropdown.Root><Dropdown.Trigger class="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm">{titles[path]||'Studio'}<SlidersHorizontal class="size-4"/></Dropdown.Trigger><Dropdown.Content align="end">{#each mobileNav as item}<Dropdown.Item onSelect={()=>void goto(item.url)}><item.icon class="size-4"/>{item.label}</Dropdown.Item>{/each}<Dropdown.Separator/><Dropdown.Item onSelect={()=>void goto('/settings/account')}>Account settings</Dropdown.Item><Dropdown.Item onSelect={()=>void goto('/credits')}>Credits</Dropdown.Item></Dropdown.Content></Dropdown.Root></nav>
  <aside class="studio-sidebar" class:collapsed={sidebarCollapsed} aria-label="Studio navigation">
    <div class="sidebar-heading">
      <button class="sidebar-toggle" onclick={toggleSidebar} aria-label={sidebarCollapsed?'Expand navigation':'Collapse navigation'} aria-expanded={!sidebarCollapsed} aria-controls="studio-navigation" title={sidebarCollapsed?'Expand navigation':'Collapse navigation'}><Menu size={22}/></button>
      {#if !sidebarCollapsed}<a href="/studio/dashboard" aria-label="VideoFlow Studio"><BrandLogo/></a>{/if}
    </div>
    <nav id="studio-navigation" aria-label="Studio" class="sidebar-links">
      {#each [
        [{url:'/studio/dashboard',label:'Overview',icon:LayoutTemplate},{url:'/projects',label:'Projects',icon:FolderOpen},...advancedNav],
        [...createNav,...outputNav,...systemNav],
        browseNav
      ] as group, index}
        {#if index>0}<div class="sidebar-divider" role="separator"></div>{/if}
        {#each group as item}
          <a href={item.url} aria-label={item.label} title={sidebarCollapsed?item.label:undefined} aria-current={active(item.url)?'page':undefined} class="sidebar-link" class:selected={active(item.url)}><item.icon size={21} strokeWidth={1.7}/>{#if !sidebarCollapsed}<span>{item.label}</span>{/if}</a>
        {/each}
      {/each}
    </nav>
    <div class="sidebar-account">{#if user}<Dropdown.Root><Dropdown.Trigger class="sidebar-link account-trigger" aria-label="Your account" title={sidebarCollapsed?'Your account':undefined}><UserRound size={21} strokeWidth={1.7}/>{#if !sidebarCollapsed}<span>{user.name||'Your account'}</span>{/if}</Dropdown.Trigger><Dropdown.Content side="top" align="start"><Dropdown.Label>Your account</Dropdown.Label>{#each [{url:'/credits',label:'Generation credits'},{url:'/premium',label:'Premium viewing'},{url:'/payments',label:'Payments'},{url:'/earnings',label:'Creator earnings'},{url:'/settings/account',label:'Settings'}] as item}<Dropdown.Item onSelect={()=>void goto(item.url)}>{item.label}</Dropdown.Item>{/each}<Dropdown.Separator/><Dropdown.Item onSelect={()=>action(async()=>{await request('/auth/logout',{});user=null;await goto('/login');})}>Sign out</Dropdown.Item></Dropdown.Content></Dropdown.Root>{:else}<a class="sidebar-link" href={signInHref} aria-label="Sign in" title="Sign in"><UserRound size={21}/>{#if !sidebarCollapsed}<span>Sign in</span>{/if}</a>{/if}</div>
  </aside>
{@render cloudContent()}
</div>{/if}

{/if}

<style>
.studio-sidebar{display:none;width:232px;flex-shrink:0;flex-direction:column;padding:12px;background:var(--background);overflow-y:auto}
.sidebar-heading{display:flex;align-items:center;gap:12px;min-height:44px;margin-bottom:20px;padding:0 4px}
.sidebar-toggle{display:grid;place-items:center;width:40px;height:40px;flex-shrink:0;border-radius:50%;color:var(--foreground)}
.sidebar-toggle:hover{background:var(--accent)}
.sidebar-links{display:flex;flex-direction:column;gap:2px}
.sidebar-link,.sidebar-account :global(.account-trigger){display:flex;align-items:center;gap:22px;width:100%;min-height:44px;padding:10px 14px;border-radius:10px;color:var(--foreground);font-size:14px;text-decoration:none;text-align:left}
.sidebar-link :global(svg){flex-shrink:0}
.sidebar-link span,.sidebar-account :global(.account-trigger span){overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.sidebar-link:hover,.sidebar-account :global(.account-trigger:hover){background:var(--accent)}
.sidebar-link.selected{background:var(--muted);font-weight:600}
.sidebar-divider{height:1px;background:var(--border);margin:12px 2px}
.sidebar-account{margin-top:auto;padding-top:20px}
.studio-sidebar.collapsed{width:76px;padding-inline:10px}
.collapsed .sidebar-heading{padding:0;justify-content:center}
.collapsed .sidebar-link,.collapsed .sidebar-account :global(.account-trigger){justify-content:center;padding:12px 0;min-height:48px}
.sidebar-toggle:focus-visible,.sidebar-link:focus-visible{outline:2px solid var(--ring);outline-offset:2px}
@media(min-width:768px){.studio-sidebar{display:flex}}
.studio-mobile-header{display:none;align-items:center;justify-content:space-between;gap:14px;padding:10px 16px;flex-shrink:0;border-bottom:1px solid var(--border)}@media(max-width:767px){.studio-mobile-header{display:flex}}</style>
