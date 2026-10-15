<script lang="ts">
  import { page } from '$app/state';
 import {selectStudioProject} from './navigation';
 import { onMount } from 'svelte';
  import EntityCanvas from '$lib/canvas/EntityCanvas.svelte';
  import { PaneGroup, Pane, Handle } from '$lib/components/ui/resizable';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Textarea } from '$lib/components/ui/textarea';
  import { Label } from '$lib/components/ui/label';
  import { MessageSquare, Send, RefreshCw, Plus, LoaderCircle } from '@lucide/svelte';
  import { request } from './api';
  import { setCanvasProject } from './legacy';
  import GenerationJobs from './GenerationJobs.svelte';
  import type { Project, Quote } from './generated';
  let projects = $state<Project[]>([]); let project = $state(''); let projectName = $state('');
  let canvas = $state<EntityCanvas>(); let error = $state(''); let busy = $state(false); let loaded = $state(false); let mobile = $state(false);
  let idea = $state(''); let clips = $state(2); let seconds = $state(5); let aspect = $state('16:9');
  let quote = $state<Quote|null>(null); let key = ''; let jobs = $state<any[]>([]); let message = $state('');
  let storageKey = '';
  async function refresh() { if (!project) return; jobs = await request(`/projects/${project}/jobs`); await canvas?.refresh(); }
  function select(id:string) { project=id; setCanvasProject(id); quote=null; jobs=[]; error=''; if(storageKey)localStorage.setItem(storageKey,id); void refresh().catch(e=>error=e.message); }
  async function createProject() { busy=true;error='';try { const p=await request('/projects',{name:projectName,description:''});projects=[p,...projects];projectName='';select(p.id); } catch(e){error=(e as Error).message;}finally{busy=false;} }
  async function prepare() { busy=true;error='';quote=null;try { quote=await request(`/projects/${project}/pipelines/quotes`,{idea,clips:Number(clips),clip_duration:Number(seconds),aspect_ratio:aspect});key=crypto.randomUUID(); }catch(e){error=(e as Error).message;}finally{busy=false;} }
  async function generate(){if(!quote)return;busy=true;error='';try{await request('/generation/jobs',{quote_id:quote.id},'POST',{'Idempotency-Key':key});quote=null;message='Your video is processing. The screenplay, scenes and shots will appear on the canvas.';await refresh();}catch(e){error=(e as Error).message;}finally{busy=false;}}
  onMount(()=>{
    let mounted=true;const media=window.matchMedia('(max-width: 767px)');const resize=()=>mobile=media.matches;resize();media.addEventListener('change',resize);
    void Promise.all([request('/me'),request('/projects')]).then(([user,p])=>{if(!mounted)return;projects=p;storageKey=`videoflow.cloud.project.${user.id}`;const saved=localStorage.getItem(storageKey);select(selectStudioProject(projects,page.url.searchParams.get('project'),saved));}).catch(e=>error=e.message).finally(()=>loaded=true);
    return()=>{mounted=false;media.removeEventListener('change',resize);setCanvasProject('');};
  });
</script>
<div class="flex h-full min-h-0 flex-col bg-background text-foreground">
  <header class="flex min-h-12 flex-wrap items-center gap-3 border-b border-border px-4 py-2">
    <label for="canvas-project" class="text-sm font-medium">Project</label>
    <select id="canvas-project" class="h-8 max-w-64 rounded-md border border-input bg-background px-2 text-sm" value={project} onchange={e=>select(e.currentTarget.value)} disabled={!projects.length}><option value="" disabled>Select a project</option>{#each projects as p}<option value={p.id}>{p.name}</option>{/each}</select>
    <Button size="sm" variant="ghost" onclick={()=>refresh().catch(e=>error=e.message)} disabled={!project}><RefreshCw class="size-4"/><span class="sr-only">Refresh canvas</span></Button>
    <a href={`/studio/editor?project=${encodeURIComponent(project)}`} class="ml-auto text-sm text-muted-foreground underline underline-offset-4">Open editor</a><a href="/studio/dashboard" class=" text-sm text-muted-foreground underline underline-offset-4">Studio overview</a>
  </header>
  {#if error}<div role="alert" class="border-b border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</div>{/if}
  {#if !loaded}<div class="grid flex-1 place-items-center"><LoaderCircle class="size-5 animate-spin"/></div>
  {:else if !project}<div class="grid flex-1 place-items-center p-6"><form class="grid w-full max-w-sm gap-4" onsubmit={e=>{e.preventDefault();void createProject();}}><h1 class="text-xl font-semibold">Make your first video</h1><p class="text-sm text-muted-foreground">Create a project to start arranging scenes, characters and shots on your canvas.</p><Label for="canvas-project-name">Project name</Label><Input id="canvas-project-name" bind:value={projectName} maxlength={120} required/><Button type="submit" disabled={busy}><Plus class="size-4"/>Create project</Button></form></div>
  {:else}
    <PaneGroup direction={mobile?'vertical':'horizontal'} class="min-h-0 flex-1">
      <Pane defaultSize={mobile?55:72} minSize={35}><div class="relative h-full min-h-80">{#key project}<EntityCanvas bind:this={canvas} onselect={()=>{}}/>{/key}</div></Pane>
      <Handle withHandle/>
      <Pane defaultSize={mobile?45:28} minSize={20}><aside class="flex h-full flex-col border-l border-border" aria-label="Video assistant">
        <header class="flex items-center gap-2 border-b border-border px-4 py-3 text-sm font-medium"><MessageSquare class="size-4"/>VideoFlow assistant</header>
        <div class="flex-1 overflow-y-auto p-4"><p class="mb-5 text-sm text-muted-foreground">Describe your story. Review the credit quote before VideoFlow writes the screenplay, generates the clips and assembles your video.</p>
          {#if message}<p role="status" class="mb-4 rounded-md bg-muted p-3 text-sm">{message}</p>{/if}
          <form class="grid gap-4" onsubmit={e=>{e.preventDefault();void prepare();}}>
            <div class="grid gap-2"><Label for="canvas-idea">Your story</Label><Textarea id="canvas-idea" rows={5} bind:value={idea} required maxlength={8000} placeholder="A small idea, a character, a world…" oninput={()=>quote=null}/></div>
            <div class="grid grid-cols-2 gap-3"><div class="grid gap-2"><Label for="canvas-clips">Scenes</Label><Input id="canvas-clips" type="number" min={1} max={4} bind:value={clips} oninput={()=>quote=null}/></div><div class="grid gap-2"><Label for="canvas-seconds">Seconds per scene</Label><Input id="canvas-seconds" type="number" min={5} max={10} bind:value={seconds} oninput={()=>quote=null}/></div></div>
            <div class="grid gap-2"><Label for="canvas-aspect">Aspect ratio</Label><select id="canvas-aspect" bind:value={aspect} onchange={()=>quote=null} class="h-9 rounded-md border border-input bg-background px-2 text-sm"><option>16:9</option><option>9:16</option><option>1:1</option></select></div>
            <p class="text-xs text-muted-foreground">Up to 30 seconds total. Models are managed by VideoFlow. Generation credits are separate from Premium.</p>
            <Button type="submit" disabled={busy || clips*seconds>30}><Send class="size-4"/>{busy?'Preparing…':'Get credit quote'}</Button>
          </form>
          {#if quote}<section class="mt-4 grid gap-3 rounded-lg border border-border p-4" aria-label="Confirm generation"><strong class="text-sm">{quote.credits.toLocaleString()} credits</strong><p class="text-sm text-muted-foreground">Credits are reserved when you confirm and charged after the finished video succeeds.</p><Button disabled={busy} onclick={generate}>Confirm and generate</Button><Button variant="ghost" onclick={()=>quote=null}>Cancel quote</Button></section>{/if}
          <GenerationJobs {jobs} onrefresh={refresh}/>
        </div>
      </aside></Pane>
    </PaneGroup>
  {/if}
</div>
