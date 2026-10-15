<script lang="ts">
  import { onMount } from 'svelte';
  import { beforeNavigate } from '$app/navigation';
  import { page } from '$app/state';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Textarea } from '$lib/components/ui/textarea';
  import * as Tabs from '$lib/components/ui/tabs';
  import * as Card from '$lib/components/ui/card';
  import * as Dialog from '$lib/components/ui/dialog';
  import { FileText, Users, ListVideo, Image, Captions, Plus, Save, Trash2, LoaderCircle, FolderOpen, ArrowUpRight, Sparkles } from '@lucide/svelte';
  import Storyboard from './Storyboard.svelte';
  import { request } from './api';
  import type { Project } from './generated';

  type Kind = 'storyboard' | 'scripts' | 'characters' | 'scenes' | 'assets' | 'shots' | 'captions';
  interface Entity { id:string; project_id:string; scene_id?:string; kind:string; title:string; content:string; description:string; prompt:string; object_key?:string; duration:number; position:number; metadata:Record<string,any>; created_at:string; }
  interface Segment { start:number; end:number; text:string; }
  const sections = [
    {value:'storyboard',label:'Storyboard',singular:'storyboard',icon:Sparkles,help:'Plan, generate, and assemble a complete project video.'},
    {value:'scripts',label:'Scripts',singular:'script',icon:FileText,help:'Write and refine the words behind your video.'},
    {value:'characters',label:'Characters',singular:'character',icon:Users,help:'Keep your cast and their appearance consistent.'},
    {value:'scenes',label:'Scenes',singular:'scene',icon:ListVideo,help:'Shape your story one scene at a time.'},
    {value:'shots',label:'Shots',singular:'shot',icon:ListVideo,help:'Plan the individual shots inside a scene.'},
    {value:'assets',label:'Assets',singular:'asset',icon:Image,help:'Organize references to media in your private workspace.'},
    {value:'captions',label:'Captions',singular:'caption',icon:Captions,help:'Edit timed captions for a completed generation.'}
  ];
  let projects = $state<Project[]>([]);
  let projectId = $state('');
  let kind = $state<string>('scripts');
  let initialLoading = $state(true);
  let loading = $state(false);
  let saving = $state(false);
  let loadFailed = $state(false);
  let importInput=$state<HTMLInputElement>(); let uploadInput=$state<HTMLInputElement>();
  let error = $state('');
  let notice = $state('');
  let entities = $state<Entity[]>([]);
  let selected = $state<Entity | null>(null);
  let formOpen = $state(false);
  let dirty = $state(false);
  let entityName = $state('');
  let description = $state('');
  let text = $state('');
  let appearance = $state('');
  let scenePrompt = $state('');
  let sceneOrder = $state(1);
  let sceneDuration = $state(5);
  let scenes = $state<Entity[]>([]);
  let sceneId = $state('');
  let assetKey = $state('');
  let assetType = $state('image');
  let deleteOpen = $state(false);
  let pendingDelete = $state<Entity | null>(null);
  let switchOpen = $state(false);
  let pendingSwitch = $state<{project:string;kind:string} | null>(null);
  let loadedProject = '';
  let loadedKind = '';
  let sequence = 0;
  let jobs = $state<any[]>([]);
  let jobId = $state('');
  let captions = $state<Segment[]>([]);
  let captionDuration = $state<number | null>(null);
  let captionsLoaded = $state(false);
  const current = $derived(sections.find(section=>section.value===kind) || sections[0]);
  const list = (value:any):any[] => Array.isArray(value) ? value : value?.items || value?.entities || value?.jobs || [];

  beforeNavigate((navigation)=>{if(dirty && !navigation.willUnload && !window.confirm('Discard your unsaved studio changes?'))navigation.cancel();});
  onMount(() => {
    const requestedTab=page.url.searchParams.get('tab');
    if(requestedTab && sections.some(section=>section.value===requestedTab))kind=requestedTab;
    void request('/projects').then(value=>{
      projects=list(value);
      const requested=page.url.searchParams.get('project');
      projectId=projects.find(project=>project.id===requested)?.id || projects[0]?.id || '';
    }).catch(e=>error=e.message).finally(()=>initialLoading=false);
  });
  $effect(()=>{
    if(initialLoading || !projectId) return;
    const nextProject=projectId;
    const nextKind=kind;
    if(nextProject===loadedProject && nextKind===loadedKind) return;
    if(dirty) {
      pendingSwitch={project:nextProject,kind:nextKind};
      projectId=loadedProject;
      kind=loadedKind;
      switchOpen=true;
      return;
    }
    loadedProject=nextProject;
    loadedKind=nextKind;
    void load(nextProject,nextKind as Kind);
  });
  function requestKind(next:string){if(next===kind)return;if(dirty){pendingSwitch={project:projectId,kind:next};switchOpen=true;return;}kind=next;}
  function discardAndSwitch() {
    if(!pendingSwitch)return;
    dirty=false; projectId=pendingSwitch.project;kind=pendingSwitch.kind;
    pendingSwitch=null;switchOpen=false;
  }
  async function load(project=projectId,section=kind as Kind) {
    const version=++sequence;
    loading=true;loadFailed=false;error='';notice='';formOpen=false;selected=null;entities=[];dirty=false;captionsLoaded=false;jobId='';captions=[];
    if(section==='storyboard'){loading=false;return;}
    try {
      const result=await request(section==='captions' ? '/jobs' : `/projects/${encodeURIComponent(project)}/${section==='shots' ? 'scenes' : section}`);
      if(version!==sequence)return;
      if(section==='shots'){scenes=list(result);sceneId=scenes[0]?.id || '';const shots=sceneId ? list(await request(`/projects/${encodeURIComponent(project)}/scenes/${encodeURIComponent(sceneId)}/shots`)) : [];if(version!==sequence)return;entities=shots;}
      else if(section==='captions')jobs=list(result).filter(job=>['completed','succeeded','complete'].includes(job.status) && (!job.project_id || job.project_id===project));
      else entities=list(result).sort((a,b)=>section==='scenes' ? (a.position || 0)-(b.position || 0) : 0);
    } catch(e) {if(version===sequence){error=(e as Error).message;loadFailed=true;}}
    finally {if(version===sequence)loading=false;}
  }
  function edit(entity:Entity | null) {
    if(dirty && !window.confirm('Discard your unsaved changes?'))return;
    selected=entity;entityName=entity?.title || '';description=entity?.description || '';
    text=entity?.content || '';appearance=entity?.content || '';scenePrompt=entity?.prompt || '';sceneOrder=entity?.position ?? entities.length+1;sceneDuration=entity?.duration || 5;assetKey=entity?.object_key || '';assetType=entity?.metadata?.type || 'image';
    formOpen=true;dirty=false;notice='';error='';
  }
  async function save() {
    saving=true;error='';notice='';
    const section=kind;
    const project=projectId;
    const resource=selected?.id;
    try {
      const fields:Record<string,unknown>={title:entityName.trim(),description,metadata:{...selected?.metadata}};
      if(section==='scripts')fields.content=text;
      else if(section==='characters')fields.content=appearance;
      else if(section==='scenes'||section==='shots')Object.assign(fields,{prompt:scenePrompt,position:Number(sceneOrder),duration:Number(sceneDuration)});
      else {fields.metadata={...selected?.metadata,type:assetType};if(!resource)fields.object_key=assetKey.trim();}
      const base=`/projects/${encodeURIComponent(project)}`;
      const endpoint=section==='shots' ? `${base}/scenes/${encodeURIComponent(sceneId)}/shots${resource ? '/'+encodeURIComponent(resource) : ''}` : `${base}/${section}${resource ? '/'+encodeURIComponent(resource) : ''}`;
      const result=await request(endpoint,fields,resource ? 'PATCH' : 'POST');
      const saved=result.entity || result;
      // Fetch the authoritative collection; no optimistic or invented records.
      entities=list(await request(section==='shots' ? `/projects/${encodeURIComponent(project)}/scenes/${encodeURIComponent(sceneId)}/shots` : `/projects/${encodeURIComponent(project)}/${section}`));
      selected=entities.find(entity=>entity.id===saved.id) || entities.find(entity=>entity.id===resource) || null;
      dirty=false;notice=`${section==='assets' ? 'Asset reference' : current.singular.charAt(0).toUpperCase()+current.singular.slice(1)} saved.`;
      if(!selected)formOpen=false;
    } catch(e) {error=(e as Error).message;}
    finally {saving=false;}
  }
  async function remove() {
    if(!pendingDelete)return;
    saving=true;error='';
    try {
      const endpoint=kind==='shots' ? `/projects/${encodeURIComponent(projectId)}/scenes/${encodeURIComponent(pendingDelete.scene_id || sceneId)}/shots/${encodeURIComponent(pendingDelete.id)}` : `/projects/${encodeURIComponent(projectId)}/${kind}/${encodeURIComponent(pendingDelete.id)}`;
      await request(endpoint,undefined,'DELETE');
      entities=entities.filter(entity=>entity.id!==pendingDelete?.id);
      if(selected?.id===pendingDelete.id){selected=null;formOpen=false;dirty=false;}
      notice='Item deleted.';deleteOpen=false;pendingDelete=null;
    } catch(e) {error=(e as Error).message;deleteOpen=false;}
    finally {saving=false;}
  }
  async function loadCaptions() {
    if(!jobId)return;
    if(dirty && !window.confirm('Discard your unsaved caption changes?'))return;
    loading=true;error='';notice='';captionsLoaded=false;
    try {
      const value=await request(`/jobs/${encodeURIComponent(jobId)}/captions`);
      captions=(value.segments || []).map((segment:Segment)=>({...segment}));
      captionDuration=value.duration ?? value.duration_seconds ?? null;captionsLoaded=true;dirty=false;
    } catch(e) {error=(e as Error).message;}
    finally {loading=false;}
  }
  async function saveCaptions() {
    saving=true;error='';notice='';
    try {
      const segments=captions.map(segment=>({start:Number(segment.start),end:Number(segment.end),text:segment.text.trim()}));
      if(segments.some((segment,index)=>!Number.isFinite(segment.start)||!Number.isFinite(segment.end)||segment.start<0||segment.end<=segment.start||!segment.text||(captionDuration!=null&&segment.end>captionDuration)||(index>0&&segment.start<segments[index-1].end)))throw new Error('Each caption needs text and a valid time range. Keep captions in order, without overlaps, and inside the video duration.');
      await request(`/jobs/${encodeURIComponent(jobId)}/captions`,{segments},'PUT');
      dirty=false;notice='Captions saved. Existing published video versions remain unchanged.';
    } catch(e) {error=(e as Error).message;}
    finally {saving=false;}
  }
  async function downloadProject() {
    saving=true;error='';
    try {
      const value=await request(`/projects/${encodeURIComponent(projectId)}/export`);
      const blob=new Blob([JSON.stringify(value,null,2)],{type:'application/json'});
      const objectUrl=URL.createObjectURL(blob);
      const anchor=document.createElement('a');anchor.href=objectUrl;anchor.download=`videoflow-project-${projectId}.json`;anchor.click();
      setTimeout(()=>URL.revokeObjectURL(objectUrl),1000);
    } catch(e){error=(e as Error).message;}finally{saving=false;}
  }
  async function importProject(file:File | undefined) {
    if(!file)return;
    saving=true;error='';notice='';
    try {
      if(file.size>1000000)throw new Error('Choose a project JSON file smaller than 1 MB.');
      let value:any;try{value=JSON.parse(await file.text());}catch{throw new Error('This file is not valid JSON.');}
      if(value.version!==1||!value.project||!Array.isArray(value.entities))throw new Error('Choose a VideoFlow version 1 project export.');
      const result=await request('/projects/import',value);
      projects=list(await request('/projects'));
      dirty=false;projectId=result.id || result.project?.id || result.project_id || projects.at(-1)?.id || projectId;
      notice='Project imported into your private workspace.';
    }catch(e){error=(e as Error).message;}finally{saving=false;if(importInput)importInput.value='';}
  }
  async function uploadAsset(file:File | undefined) {
    if(!file)return;
    saving=true;error='';notice='';
    try {
      if(file.size>100000000)throw new Error('Choose a media file smaller than 100 MB.');
      if(!file.type)throw new Error('This file has no recognized media type. Choose an image, video, or audio file.');
      const grant=await request('/assets/upload-grant',{filename:file.name,content_type:file.type,bytes:file.size});
      if(grant.method!=='POST'||!grant.url||!grant.key||!grant.fields)throw new Error('The upload service returned an incomplete upload grant.');
      const form=new FormData();for(const [key,value] of Object.entries(grant.fields))form.append(key,String(value));form.append('file',file);
      const response=await fetch(grant.url,{method:'POST',body:form});
      if(!response.ok)throw new Error('The media upload did not complete. Please try again.');
      assetKey=grant.key;assetType=file.type.startsWith('video/') ? 'video' : file.type.startsWith('audio/') ? 'audio' : 'image';
      if(!entityName)entityName=file.name;dirty=true;notice='Media uploaded. Save this asset reference to attach it to your project. Uploaded files remain subject to validation.';
    }catch(e){error=(e as Error).message;}finally{saving=false;if(uploadInput)uploadInput.value='';}
  }
  function beforeUnload(event:BeforeUnloadEvent) {if(dirty){event.preventDefault();event.returnValue='';}}
</script>

<svelte:window onbeforeunload={beforeUnload}/>
<div class="studio-workspace">
  <div class="page-heading studio-heading"><div><span class="eyebrow">A LITTLE MORE CREATIVE CONTROL</span><h1>Behind the story.</h1><p>Your scripts, cast, scenes, and media. All in one private workspace.</p></div><Button href="/create" variant="outline">Create video <ArrowUpRight size={15}/></Button></div>
  {#if error}<div class="cloud-alert" role="alert"><span>{error}</span><Button variant="ghost" size="sm" onclick={()=>{if(!dirty||window.confirm('Discard unsaved changes and reload?'))void load();}}>Reload</Button></div>{/if}
  {#if notice}<div class="cloud-notice" role="status">{notice}</div>{/if}
  <div class="studio-transfer"><input class="studio-file-input" type="file" accept=".json,application/json" bind:this={importInput} aria-label="Import project JSON" onchange={(event)=>void importProject(event.currentTarget.files?.[0])}/><Button variant="outline" size="sm" disabled={initialLoading||saving||dirty} onclick={()=>importInput?.click()}>Import project JSON</Button><Button variant="outline" size="sm" disabled={saving||dirty||!projectId} onclick={downloadProject}>Download project JSON</Button><span class="subtle">Exports contain saved project data. Media files stay in your workspace.</span></div>
  {#if initialLoading}<div class="cloud-loading" role="status"><LoaderCircle size={23} class="animate-spin"/>Opening your workspace…</div>
  {:else if !projects.length}<div class="cloud-empty compact"><FolderOpen size={35}/><h2>A project is your starting point.</h2><p>Create a project to organize scripts, characters, scenes, and assets.</p><Button href="/projects">Create a project <Plus size={15}/></Button></div>
  {:else}
    <div class="studio-project"><div><label for="studio-project">Working in</label><select id="studio-project" value={projectId} disabled={saving} onchange={(event)=>{const next=event.currentTarget.value;if(dirty){pendingSwitch={project:next,kind};switchOpen=true;event.currentTarget.value=projectId;return;}projectId=next;}}>{#each projects as project}<option value={project.id}>{project.name}</option>{/each}</select></div><span class="subtle">Private workspace · {dirty ? 'Unsaved changes' : 'Changes are saved explicitly'}</span></div>
    <Tabs.Root bind:value={() => kind, requestKind}>
      <div class="studio-tabs"><Tabs.List variant="line" aria-label="Project resources">{#each sections as section}<Tabs.Trigger value={section.value} disabled={saving}><section.icon size={15}/>{section.label}</Tabs.Trigger>{/each}</Tabs.List></div>
      {#each sections as section}<Tabs.Content value={section.value}>{#if kind===section.value}
        {#if kind!=='storyboard'}<div class="section-heading studio-section-heading"><div><h2>{section.label}</h2><p class="subtle">{section.help}</p></div>{#if kind!=='captions'}<Button disabled={loading||saving||(kind==='shots'&&!sceneId)} onclick={()=>edit(null)}><Plus size={15}/>New {section.singular}</Button>{/if}</div>{/if}
        {#if loading}<div class="cloud-loading studio-loading" role="status"><LoaderCircle size={20} class="animate-spin"/>Loading {section.label.toLowerCase()}…</div>{/if}
        {#if kind==='storyboard'}<Storyboard {projectId} onDirtyChange={(value)=>dirty=value}/>
        {:else if loadFailed}<div class="studio-list-empty"><FolderOpen size={27}/><h3>This resource could not be loaded.</h3><p>The server may be unavailable or this project may no longer be accessible. Reload to check your current access.</p><Button variant="outline" onclick={()=>load()}>Reload resources</Button></div>
        {:else if kind==='captions'}
          <div class="caption-job-select"><div><label for="caption-job">Completed generation</label><select id="caption-job" bind:value={jobId} disabled={saving||loading}><option value="">Choose a completed generation</option>{#each jobs as job}<option value={job.id}>{job.name || job.prompt || job.id}</option>{/each}</select></div><Button variant="outline" disabled={!jobId||saving||loading} onclick={loadCaptions}>Load captions</Button></div>
          {#if !loading && !jobs.length}<p class="quiet-empty">Completed video generations will appear here when they are ready for captions.</p>{/if}
          {#if captionsLoaded}<div class="caption-editor"><div class="section-heading"><p class="subtle">Times are in seconds{captionDuration!=null ? ` · Video duration: ${captionDuration}s` : ''}. Captions must not overlap.</p><Button size="sm" variant="outline" disabled={saving} onclick={()=>{const last=captions.at(-1)?.end || 0;captions=[...captions,{start:last,end:Math.min(last+2,captionDuration??last+2),text:''}];dirty=true;}}><Plus size={14}/>Add caption</Button></div>
            {#each captions as segment,index}<div class="caption-row"><span class="caption-number">{String(index+1).padStart(2,'0')}</span><div><label for={`caption-start-${index}`}>Start (s)</label><Input id={`caption-start-${index}`} type="number" min={0} step="0.01" bind:value={segment.start} oninput={()=>dirty=true} disabled={saving}/></div><div><label for={`caption-end-${index}`}>End (s)</label><Input id={`caption-end-${index}`} type="number" min={0} step="0.01" bind:value={segment.end} oninput={()=>dirty=true} disabled={saving}/></div><div class="caption-text"><label for={`caption-text-${index}`}>Caption text</label><Textarea id={`caption-text-${index}`} bind:value={segment.text} rows={2} oninput={()=>dirty=true} disabled={saving}/></div><Button size="icon-sm" variant="ghost" aria-label={`Remove caption ${index+1}`} disabled={saving} onclick={()=>{captions=captions.filter((_,i)=>i!==index);dirty=true;}}><Trash2 size={15}/></Button></div>{:else}<p class="quiet-empty">No captions saved. Add your first timed line.</p>{/each}
            <div class="studio-save-row"><span class="subtle">{dirty ? 'You have unsaved changes.' : 'Captions are up to date.'}</span><Button disabled={saving||!dirty} onclick={saveCaptions}>{#if saving}<LoaderCircle size={15} class="animate-spin"/>{:else}<Save size={15}/>{/if}Save captions</Button></div></div>{/if}
        {:else if !loading}
          {#if kind==='shots'}<div class="caption-job-select"><div><label for="shot-scene">Scene</label><select id="shot-scene" value={sceneId} disabled={saving} onchange={async(event)=>{if(dirty&&!window.confirm('Discard unsaved changes?'))return;sceneId=event.currentTarget.value;loading=true;error='';try{entities=list(await request(`/projects/${encodeURIComponent(projectId)}/scenes/${encodeURIComponent(sceneId)}/shots`));selected=null;formOpen=false;dirty=false;}catch(e){error=(e as Error).message;}finally{loading=false;}}}>{#each scenes as scene}<option value={scene.id}>{scene.title}</option>{/each}</select>{#if !scenes.length}<p class="field-help">Create a scene before adding shots.</p>{/if}</div></div>{/if}
          <div class="studio-edit-layout"><section class="studio-resource-list" aria-label={`${section.label} in this project`}>
            {#each entities as entity}<Card.Root class={`studio-resource ${selected?.id===entity.id ? 'studio-selected' : ''}`}><button class="studio-resource-open" onclick={()=>edit(entity)} disabled={saving}><div><section.icon size={19}/><h3>{entity.title}</h3></div><p>{entity.description || (kind==='scripts' ? entity.content : kind==='characters' ? entity.content : (kind==='scenes'||kind==='shots') ? entity.prompt : entity.metadata?.type) || 'No description yet.'}</p>{#if kind==='scenes'}<small>Scene {entity.position ?? '—'}</small>{/if}</button><Button variant="ghost" size="icon-sm" aria-label={`Delete ${entity.title}`} disabled={saving} onclick={()=>{pendingDelete=entity;deleteOpen=true;}}><Trash2 size={14}/></Button></Card.Root>{:else}<div class="studio-list-empty"><section.icon size={27}/><h3>No {section.label.toLowerCase()} yet.</h3><p>Add a {section.singular} to start shaping this project.</p></div>{/each}
          </section><Card.Root class="studio-editor"><Card.Header><Card.Title>{formOpen ? `${selected ? 'Edit' : 'New'} ${section.singular}` : 'Make space for the details.'}</Card.Title><Card.Description>{formOpen ? 'Save your changes when you’re ready.' : `Choose a ${section.singular}, or add a new one to this project.`}</Card.Description></Card.Header><Card.Content>
            {#if formOpen}<form onsubmit={(event)=>{event.preventDefault();void save();}} oninput={()=>dirty=true}><label for="entity-name">Name</label><Input id="entity-name" bind:value={entityName} required maxlength={180} disabled={saving}/><label for="entity-description">Description <span class="subtle">(optional)</span></label><Textarea id="entity-description" bind:value={description} rows={2} maxlength={4000} disabled={saving}/>
              {#if kind==='scripts'}<label for="script-text">Script</label><Textarea id="script-text" bind:value={text} maxlength={100000} rows={12} placeholder="Write your story, narration, or dialogue…" disabled={saving}/>
              {:else if kind==='characters'}<label for="character-appearance">Appearance & defining details</label><Textarea id="character-appearance" bind:value={appearance} maxlength={100000} rows={7} placeholder="Describe this character’s appearance, wardrobe, and distinctive traits…" disabled={saving}/>
              {:else if kind==='scenes'||kind==='shots'}<label for="scene-order">Scene order</label><Input id="scene-order" type="number" min={0} max={10000} step={1} bind:value={sceneOrder} required disabled={saving}/><label for="scene-duration">Duration (seconds)</label><Input id="scene-duration" type="number" min={1} max={3600} step={1} bind:value={sceneDuration} required disabled={saving}/><label for="scene-prompt">{kind==='shots' ? 'Shot' : 'Scene'} direction</label><Textarea id="scene-prompt" bind:value={scenePrompt} maxlength={8000} rows={7} placeholder="Describe the setting, action, camera, and mood…" disabled={saving}/>
              {:else if kind==='assets'}{#if !selected}<div class="studio-upload"><input class="studio-file-input" type="file" accept="image/*,video/*,audio/*" bind:this={uploadInput} aria-label="Upload media" onchange={(event)=>void uploadAsset(event.currentTarget.files?.[0])}/><Button variant="outline" type="button" disabled={saving} onclick={()=>uploadInput?.click()}>Upload media file</Button><p class="field-help">Image, video, or audio · Up to 100 MB. Upload availability depends on storage configuration.</p></div>{/if}<label for="asset-type">Media type</label><select id="asset-type" bind:value={assetType} disabled={saving}><option value="image">Image</option><option value="video">Video</option><option value="audio">Audio</option></select><label for="asset-key">Workspace storage key</label><Input id="asset-key" bind:value={assetKey} required placeholder="Paste a registered workspace-owned storage key" disabled={saving||!!selected}/><p class="field-help">Reference an existing private workspace asset. The server checks ownership; keys from another workspace cannot be used.</p>{/if}
              <div class="studio-save-row"><span class="subtle">{dirty ? 'Unsaved changes' : 'Saved state'}</span><Button type="submit" disabled={saving||!entityName.trim()||!dirty}>{#if saving}<LoaderCircle size={15} class="animate-spin"/>{:else}<Save size={15}/>{/if}Save {section.singular}</Button></div>
            </form>{:else}<div class="studio-editor-placeholder"><section.icon size={42}/><p>Your creative decisions belong here.</p></div>{/if}
          </Card.Content></Card.Root></div>
        {/if}
      {/if}</Tabs.Content>{/each}
    </Tabs.Root>
  {/if}
</div>

<Dialog.Root bind:open={deleteOpen}><Dialog.Content><Dialog.Header><Dialog.Title>Delete {pendingDelete?.title || 'this item'}?</Dialog.Title><Dialog.Description>This permanently removes the item from this project{kind==='scenes' ? ' and its nested shots' : ''}. This action cannot be undone.</Dialog.Description></Dialog.Header><Dialog.Footer><Button variant="outline" disabled={saving} onclick={()=>deleteOpen=false}>Keep item</Button><Button variant="destructive" disabled={saving} onclick={remove}>{saving ? 'Deleting…' : 'Delete item'}</Button></Dialog.Footer></Dialog.Content></Dialog.Root>
<Dialog.Root bind:open={switchOpen}><Dialog.Content><Dialog.Header><Dialog.Title>Leave unsaved changes?</Dialog.Title><Dialog.Description>Your edits have not been saved. Save them first, or discard them to switch your workspace view.</Dialog.Description></Dialog.Header><Dialog.Footer><Button variant="outline" onclick={()=>{switchOpen=false;pendingSwitch=null;}}>Keep editing</Button><Button variant="destructive" onclick={discardAndSwitch}>Discard changes</Button></Dialog.Footer></Dialog.Content></Dialog.Root>

<style>
  .studio-transfer{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-bottom:25px}.studio-transfer>.subtle{font-size:12px;margin-left:auto;max-width:280px}.studio-file-input{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%);white-space:nowrap}.studio-upload{padding:20px 0 0}.studio-upload p{max-width:330px}

  .studio-heading{display:flex;align-items:center;justify-content:space-between;gap:25px}.studio-project{display:flex;align-items:end;justify-content:space-between;gap:25px;padding:0 0 26px;border-bottom:1px solid #e1e5d8}.studio-project>div{max-width:360px;flex:1}.studio-project label{margin-top:0}.studio-tabs{overflow:auto;border-bottom:1px solid #e1e5d8;padding:16px 0 7px}.studio-tabs :global([data-slot=tabs-list]){height:auto;gap:15px}.studio-tabs :global([data-slot=tabs-trigger]){gap:7px;font-size:12px;padding:9px 10px}.studio-section-heading{margin-top:25px}.studio-section-heading p{margin-top:5px}.studio-edit-layout{display:grid;grid-template-columns:minmax(210px,1fr) minmax(300px,1.65fr);gap:25px;align-items:start}.studio-resource-list{display:grid;gap:12px}.studio-resource-list :global(.studio-resource){display:flex;flex-direction:row;gap:0;position:relative;padding:0;border-color:#e3e6dc;box-shadow:none;background:#fff}.studio-resource-list :global(.studio-selected){border-color:#8e9d7a;background:#f2f5ec}.studio-resource-list :global(.studio-resource>button:last-child){position:absolute;right:12px;top:14px}.studio-resource-open{padding:20px;text-align:left;width:100%;cursor:pointer;min-width:0}.studio-resource-open>div{display:flex;align-items:center;gap:11px;padding-right:28px}.studio-resource-open :global(svg){color:#646e56;flex-shrink:0}.studio-resource-open h3{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.studio-resource-open p{font-size:12px;color:#69715f;line-height:1.8;margin-top:12px;display:-webkit-box;-webkit-line-clamp:3;line-clamp:3;-webkit-box-orient:vertical;overflow:hidden;overflow-wrap:anywhere}.studio-resource-open small{display:block;margin-top:8px;font-size:12px;color:#687458}.studio-resource-open:hover{background:#f2f5eb;border-radius:12px}.studio-list-empty{padding:45px 20px;border:1px dashed #dce2d1;border-radius:12px;text-align:center;color:#636d58}.studio-list-empty :global(svg){margin:0 auto 17px}.studio-list-empty p{font-size:12px;line-height:1.8;margin-top:10px}.studio-edit-layout :global(.studio-editor){box-shadow:none;gap:0;padding:25px 0;background:white;border-color:#e2e5db}.studio-edit-layout :global(.studio-editor [data-slot=card-title]){font-size:17px;font-weight:550;letter-spacing:-.4px}.studio-edit-layout :global(.studio-editor [data-slot=card-description]){font-size:12px;line-height:1.8}.studio-edit-layout :global(.studio-editor label:first-child){margin-top:20px}.studio-save-row{display:flex;align-items:center;justify-content:space-between;gap:20px;border-top:1px solid #e4e8dc;margin-top:25px;padding-top:20px}.studio-editor-placeholder{padding:70px 0;text-align:center;color:#6a6f63}.studio-editor-placeholder :global(svg){margin:0 auto 17px}.studio-editor-placeholder p{font-size:12px}.studio-loading{min-height:200px}.caption-job-select{display:flex;align-items:end;gap:17px;margin-bottom:25px}.caption-job-select>div{flex:1;max-width:500px}.caption-job-select label{margin-top:0}.caption-row{display:flex;align-items:center;gap:15px;border-bottom:1px solid #e4e7dc;padding:20px 0}.caption-row>div:not(.caption-text){width:95px;flex-shrink:0}.caption-row label{margin-top:0;font-size:12px}.caption-text{flex:1}.caption-number{font-size:12px;color:#666d5c;width:22px;flex-shrink:0}.caption-row > :global([data-slot=button]){align-self:center;margin-top:20px}.caption-editor{background:white;border:1px solid #e1e5d8;border-radius:12px;padding:25px}.caption-editor>.section-heading{margin-bottom:10px}@media(max-width:1000px){.studio-edit-layout{grid-template-columns:1fr}.studio-resource-list{grid-template-columns:1fr 1fr}.studio-tabs :global([data-slot=tabs-list]){gap:4px}.caption-row{flex-wrap:wrap;gap:12px}.caption-text{flex-basis:calc(100% - 50px);margin-left:34px}.caption-row > :global([data-slot=button]){margin-top:20px}.caption-row>div:not(.caption-text){flex:1}.caption-job-select{align-items:stretch;flex-direction:column}}@media(max-width:600px){.studio-heading{align-items:start;flex-direction:column;gap:18px}.studio-project{align-items:start;flex-direction:column;gap:10px}.studio-project>div{width:100%;max-width:none}.studio-resource-list{grid-template-columns:1fr}.studio-tabs :global([data-slot=tabs-trigger]){font-size:12px;padding-inline:7px}.studio-tabs :global([data-slot=tabs-trigger] svg){display:none}.caption-editor{padding:17px}.caption-editor>.section-heading{align-items:start;flex-direction:column;gap:10px}.studio-save-row{align-items:start;flex-direction:column;gap:12px}.studio-save-row > :global([data-slot=button]){width:100%}.studio-section-heading h2{font-size:18px}}
</style>
