<script lang="ts">
  import { onMount } from 'svelte';
  import {guardUnsaved} from './unsaved';import{confirmAction}from'./confirmation.svelte';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Textarea } from '$lib/components/ui/textarea';
  import * as Tabs from '$lib/components/ui/tabs';
  import { Sparkles, Film, ArrowRight, ArrowUp, ArrowDown, Plus, X, Save, LoaderCircle, RefreshCw, ShieldCheck, Coins, Layers, ListVideo } from '@lucide/svelte';
  import { request } from './api';
  import GenerationJobs from './GenerationJobs.svelte';
  import StoryboardGraph from './StoryboardGraph.svelte';
  import type { Quote } from './generated';

  interface Entity {id:string;title:string;prompt:string;content:string;duration:number;position:number;scene_id?:string;}
  interface SceneRow {scene:Entity;shots:Entity[];}
  let {projectId,onDirtyChange}: {projectId:string;onDirtyChange?:(dirty:boolean)=>void}=$props();
  let mode=$state('story');
  let idea=$state('');let clipCount=$state('3');let clipDuration=$state('5');let aspectRatio=$state('16:9');
  let quote=$state<Quote|null>(null);let quoteKey='';let quoteMode=$state('');
  let quoteExpired=$state(false);
  let loading=$state(true);let busy=$state(false);let saving=$state(false);
  let error=$state('');let notice=$state('');let timelineError=$state('');
  let balance=$state<any>(null);let jobs=$state<any[]>([]);let rows=$state<SceneRow[]>([]);
  let order=$state<string[]>([]);let dirty=$state(false);let selectedScene=$state('');let selectedShot=$state('');let submittedId=$state('');
  let version=0;
  const list=(value:any):any[]=>Array.isArray(value)?value:value?.items||value?.jobs||[];
  const successful=(job:any)=>['succeeded','completed','complete'].includes(job.status);
  const availableJobs=$derived(jobs.filter(successful));
  const selectedJobs=$derived(order.map(id=>availableJobs.find(job=>job.id===id)).filter(Boolean));
  const selectedRow=$derived(rows.find(row=>row.scene.id===selectedScene));
  const sceneDuration=(row:SceneRow)=>row.shots.length ? row.shots.reduce((total,shot)=>total+(Number(shot.duration)||0),0) : Number(row.scene.duration)||0;
  const totalDuration=$derived(rows.reduce((total,row)=>total+sceneDuration(row),0));
  const totalShots=$derived(rows.reduce((total,row)=>total+row.shots.length,0));
  const base=()=>`/projects/${encodeURIComponent(projectId)}`;
  const label=(job:any)=>job.title||job.prompt||job.name||`${job.kind==='pipeline'?'Complete video':'Clip'} · ${job.id.slice(0,8)}`;
  let lastMode='story';
  $effect(()=>{const value=mode;if(value!==lastMode){quote=null;lastMode=value;}});
  $effect(()=>{onDirtyChange?.(dirty);});
  $effect(()=>{const id=projectId;if(id)void initialize(id);});
  guardUnsaved(()=>!onDirtyChange&&dirty,'You have unsaved storyboard edits.');
  onMount(()=>{const timer=setInterval(()=>{quoteExpired=!!quote&&Date.parse(quote.expires_at)<=Date.now();},1000);return()=>clearInterval(timer);});
  async function initialize(id:string){
    const current=++version;loading=true;error='';notice='';quote=null;order=[];rows=[];dirty=false;submittedId='';
    try{
      const [allJobs,creditBalance]=await Promise.all([request(`/projects/${encodeURIComponent(id)}/jobs`),request('/credits').catch(()=>null)]);
      if(current!==version)return;jobs=list(allJobs).filter(job=>job.project_id===id);balance=creditBalance;
      await loadTimeline(id,current);
    }catch(e){if(current===version)error=(e as Error).message;}
    finally{if(current===version)loading=false;}
  }
  async function loadTimeline(id=projectId,current=version){
    timelineError='';
    try{
      const scenes:Entity[]=list(await request(`/projects/${encodeURIComponent(id)}/scenes`)).sort((a,b)=>a.position-b.position);
      const result:SceneRow[]=[];
      // Bound parallel requests for projects with larger storyboards.
      for(let offset=0;offset<scenes.length;offset+=4){
        const batch=await Promise.all(scenes.slice(offset,offset+4).map(async scene=>({scene,shots:list(await request(`/projects/${encodeURIComponent(id)}/scenes/${encodeURIComponent(scene.id)}/shots`)).sort((a,b)=>a.position-b.position)})));
        result.push(...batch);if(current!==version)return;
      }
      if(current!==version)return;rows=result;selectedShot='';selectedScene=result.some(row=>row.scene.id===selectedScene)?selectedScene:result[0]?.scene.id||'';dirty=false;
    }catch(e){if(current===version)timelineError=(e as Error).message;}
  }
  async function refreshJobs(){
    const id=projectId;const current=version;
    const value=list(await request(`/projects/${encodeURIComponent(id)}/jobs`)).filter(job=>job.project_id===id);
    if(current!==version)return;
    const newOutput=value.some(job=>successful(job)&&!jobs.some(previous=>previous.id===job.id&&successful(previous)));
    jobs=value;
    if(newOutput&&!dirty)await loadTimeline(id,current);
    balance=await request('/credits').catch(()=>balance);
  }
  async function reviewQuote(){
    busy=true;error='';notice='';quote=null;
    try{
      const requestedMode=mode;
      quote=await request(`${base()}/${mode==='story'?'pipelines':'assembly'}/quotes`,mode==='story'?{idea:idea.trim(),clips:Number(clipCount),clip_duration:Number(clipDuration),aspect_ratio:aspectRatio}:{job_ids:order});
      quoteKey=crypto.randomUUID();quoteMode=requestedMode;quoteExpired=Date.parse(quote!.expires_at)<=Date.now();
    }catch(e){error=(e as Error).message;}finally{busy=false;}
  }
  async function confirm(){
    if(!quote||quoteExpired)return;
    const quoteId=quote.id;busy=true;error='';notice='';
    try{
      const result=await request('/generation/jobs',{quote_id:quoteId},'POST',{'Idempotency-Key':quoteKey});
      submittedId=result.id;quote=null;await refreshJobs();notice='Your video job is saved. Follow its stages below; you can leave this page while it runs.';
    }catch(e){error=(e as Error).message;}finally{busy=false;}
  }
  function addClip(id:string){if(id&&!order.includes(id)&&order.length<8){order=[...order,id];quote=null;}}
  function moveClip(index:number,direction:number){const target=index+direction;if(target<0||target>=order.length)return;const updated=[...order];[updated[index],updated[target]]=[updated[target],updated[index]];order=updated;quote=null;}
  function moveScene(index:number,direction:number){const target=index+direction;if(target<0||target>=rows.length)return;const updated=[...rows];[updated[index],updated[target]]=[updated[target],updated[index]];rows=updated;dirty=true;}
  function moveShot(row:SceneRow,index:number,direction:number){const target=index+direction;if(target<0||target>=row.shots.length)return;const updated=[...row.shots];[updated[index],updated[target]]=[updated[target],updated[index]];row.shots=updated;dirty=true;}
  async function saveTimeline(){
    saving=true;error='';notice='';
    try{
      for(const row of rows){
        if(!row.scene.title.trim()||!Number.isFinite(Number(row.scene.duration))||Number(row.scene.duration)<0||Number(row.scene.duration)>3600)throw new Error('Each scene needs a title and a duration between 0 and 3600 seconds.');
        for(const shot of row.shots)if(!shot.title.trim()||!Number.isFinite(Number(shot.duration))||Number(shot.duration)<0||Number(shot.duration)>3600)throw new Error('Each shot needs a title and a duration between 0 and 3600 seconds.');
      }
      for(const [index,row]of rows.entries()){
        await request(`${base()}/scenes/${encodeURIComponent(row.scene.id)}`,{title:row.scene.title.trim(),prompt:row.scene.prompt,duration:Number(row.scene.duration),position:index+1},'PATCH');
        for(const [shotIndex,shot]of row.shots.entries())await request(`${base()}/scenes/${encodeURIComponent(row.scene.id)}/shots/${encodeURIComponent(shot.id)}`,{title:shot.title.trim(),prompt:shot.prompt,duration:Number(shot.duration),position:shotIndex+1},'PATCH');
      }
      await loadTimeline();dirty=false;notice='Storyboard order and timing saved. Existing generated clips remain unchanged.';
    }catch(e){error=`${(e as Error).message} Some edits may have been saved. Reload the storyboard to review the current server state before retrying.`;}
    finally{saving=false;}
  }
  function beforeUnload(event:BeforeUnloadEvent){if(dirty){event.preventDefault();event.returnValue='';}}
</script>
<svelte:window onbeforeunload={beforeUnload}/>
<div class="storyboard-workflow">
  {#if error}<div class="cloud-alert" role="alert">{error}</div>{/if}{#if notice}<div class="cloud-notice" role="status">{notice}</div>{/if}
  {#if loading}<div class="cloud-loading"><LoaderCircle size={22} class="animate-spin"/>Opening your storyboard…</div>{:else}
    <details class="generation-disclosure" open={rows.length===0}><summary>Generate or assemble video <span>Idea, format and credit cost</span></summary>
    <div class="workflow-heading"><div><h2>Make a complete video.</h2><p>Generate a screenplay, create its clips, and assemble a complete video. Or bring together clips you have already made.</p></div><div class="workflow-balance"><Coins size={17}/>{#if balance}<span><strong>{balance.available?.toLocaleString() ?? '—'}</strong> available credits<small>{balance.reserved ?? '—'} reserved</small></span>{:else}<span>Balance unavailable<small>Costs are checked by the server.</small></span>{/if}</div></div>
    {#if balance?.restricted}<div class="cloud-alert" role="alert"><div><strong>Generation is restricted on this account.</strong><p>Outstanding credit debt: {balance.debt ?? '—'}. Review your <a href="/credits">credit activity</a> before starting another job.</p></div></div>{/if}
    <Tabs.Root bind:value={mode}><Tabs.List aria-label="Video creation method"><Tabs.Trigger value="story" disabled={busy} onclick={()=>quote=null}><Sparkles size={15}/>Story to video</Tabs.Trigger><Tabs.Trigger value="assembly" disabled={busy} onclick={()=>quote=null}><Layers size={15}/>Assemble clips</Tabs.Trigger></Tabs.List>
    <Tabs.Content value="story"><form class="workflow-compose" onsubmit={(event)=>{event.preventDefault();void reviewQuote();}} oninput={()=>quote=null}><div class="workflow-main-field"><label for="story-idea">Your story idea</label><Textarea id="story-idea" bind:value={idea} rows={5} minlength={10} maxlength={12000} required disabled={busy} placeholder="An old lighthouse keeper finds a message that has travelled through time. A cinematic short with a quiet, hopeful ending…"/><p class="field-help">Describe the story, setting, characters, and visual mood. 6eek plans the scenes and assembles their generated clips. Up to 30 seconds total.</p></div><div class="workflow-options"><div><label for="story-clips">Scenes / clips</label><select id="story-clips" bind:value={clipCount} disabled={busy} onchange={()=>{if(Number(clipCount)*Number(clipDuration)>30)clipDuration=String(Math.floor(30/Number(clipCount)));quote=null;}}><option value="1">1 clip</option><option value="2">2 clips</option><option value="3">3 clips</option><option value="4">4 clips</option></select></div><div><label for="story-duration">Each clip</label><select id="story-duration" bind:value={clipDuration} disabled={busy}><option value="5">5 seconds</option><option value="6">6 seconds</option><option value="7">7 seconds</option><option value="8" disabled={Number(clipCount)*8>30}>8 seconds</option><option value="9" disabled={Number(clipCount)*9>30}>9 seconds</option><option value="10" disabled={Number(clipCount)*10>30}>10 seconds</option></select></div><div><label for="story-frame">Frame</label><select id="story-frame" bind:value={aspectRatio} disabled={busy}><option>16:9</option><option>9:16</option><option>1:1</option></select></div><div class="workflow-length"><span>Planned length</span><strong>{Number(clipCount)*Number(clipDuration)} seconds</strong></div></div><div class="workflow-footer"><span><ShieldCheck size={15}/>Private until you publish</span><Button type="submit" disabled={busy||idea.trim().length<10||Number(clipCount)*Number(clipDuration)>30||balance?.restricted}>{#if busy}<LoaderCircle size={15} class="animate-spin"/>{:else}<Coins size={15}/>{/if}Review full video cost <ArrowRight size={15}/></Button></div></form></Tabs.Content>
    <Tabs.Content value="assembly"><form class="workflow-compose" onsubmit={(event)=>{event.preventDefault();void reviewQuote();}}><div><label for="assembly-clip">Add a completed clip from this project</label><select id="assembly-clip" value="" disabled={busy||order.length>=8} onchange={(event)=>{addClip(event.currentTarget.value);event.currentTarget.value='';}}><option value="">Choose a completed clip</option>{#each availableJobs.filter(job=>!order.includes(job.id)) as job}<option value={job.id}>{label(job)}</option>{/each}</select><p class="field-help">Choose up to 8 completed outputs. They play in the order below. Final assembly uses a 16:9 frame and normalized audio. Original clips are preserved.</p></div>{#if !availableJobs.length}<div class="assembly-empty"><Film size={24}/><p>No completed clips in this project yet. Generate a video here or choose this project in Create video.</p></div>{/if}<ol class="assembly-order">{#each selectedJobs as job,index}<li><span>{String(index+1).padStart(2,'0')}</span><strong>{label(job)}</strong><div><Button variant="ghost" size="icon-sm" type="button" aria-label={`Move clip ${index+1} earlier`} disabled={index===0||busy} onclick={()=>moveClip(index,-1)}><ArrowUp size={14}/></Button><Button variant="ghost" size="icon-sm" type="button" aria-label={`Move clip ${index+1} later`} disabled={index===order.length-1||busy} onclick={()=>moveClip(index,1)}><ArrowDown size={14}/></Button><Button variant="ghost" size="icon-sm" type="button" aria-label={`Remove clip ${index+1}`} disabled={busy} onclick={()=>{order=order.filter(id=>id!==job.id);quote=null;}}><X size={14}/></Button></div></li>{/each}</ol><div class="workflow-footer"><span>{order.length} of 8 clips selected</span><Button type="submit" disabled={busy||!order.length||balance?.restricted}><Coins size={15}/>Review assembly cost <ArrowRight size={15}/></Button></div></form></Tabs.Content></Tabs.Root>
    {#if quote}<div class="workflow-quote" aria-live="polite"><div><span class="eyebrow">{quoteMode==='story' ? 'FULL STORY-TO-VIDEO QUOTE' : 'ASSEMBLY QUOTE'}</span><strong>{quote.credits.toLocaleString()} credits</strong><p>{quoteExpired ? 'This quote has expired. Request a new cost before continuing.' : `Valid until ${new Date(quote.expires_at).toLocaleTimeString()}. Credits are reserved on confirmation and charged on final success.`}</p></div><Button disabled={busy||quoteExpired||balance?.restricted} onclick={confirm}>{#if busy}<LoaderCircle size={15} class="animate-spin"/>{/if}Confirm {quoteMode==='story'?'full video':'assembly'}<ArrowRight size={15}/></Button></div>{/if}
    </details>
    {#if jobs.length}<GenerationJobs {jobs} onrefresh={refreshJobs} focusId={submittedId}/>{/if}
    <section class="storyboard-timeline"><div class="section-heading"><div><h2>Storyboard</h2><p class="subtle">{rows.length} scenes · {totalShots} shots · {totalDuration.toFixed(1).replace(/\.0$/,'')}s planned</p></div><div class="timeline-toolbar"><Button variant="ghost" size="sm" disabled={saving} onclick={async()=>{if(!dirty||await confirmAction('Discard unsaved edits and reload the storyboard?'))void loadTimeline();}}><RefreshCw size={14}/>Reload</Button><Button size="sm" disabled={saving||!dirty} onclick={saveTimeline}>{#if saving}<LoaderCircle size={14} class="animate-spin"/>{:else}<Save size={14}/>{/if}Save storyboard</Button></div></div><p class="timeline-disclosure">Storyboard edits organize your project; they do not alter existing generated clips. Open Edit to arrange generated clips on your video timeline.</p>
    {#if timelineError}<div class="cloud-alert" role="alert">{timelineError}</div>{:else if !rows.length}<div class="timeline-empty"><ListVideo size={28}/><h3>Your storyboard starts here.</h3><p>Describe your idea above to generate scenes, or add your own scenes in project resources.</p><Button variant="outline" href={`/workspace?tab=scenes&project=${encodeURIComponent(projectId)}`}>Add scenes</Button></div>{:else}
      <div class="storyboard-surface"><StoryboardGraph {rows} {selectedScene} {selectedShot} onselect={(selection)=>{selectedScene=selection.sceneId;selectedShot=selection.shotId || '';}}/>
      {#if selectedRow}<div class="timeline-scene-editor" id="storyboard-editor"><div class="scene-order-actions"><span>Scene {rows.indexOf(selectedRow)+1} of {rows.length}</span><Button variant="ghost" size="icon-sm" aria-label="Move selected scene earlier" disabled={rows.indexOf(selectedRow)===0||saving} onclick={()=>moveScene(rows.indexOf(selectedRow),-1)}><ArrowUp size={14}/></Button><Button variant="ghost" size="icon-sm" aria-label="Move selected scene later" disabled={rows.indexOf(selectedRow)===rows.length-1||saving} onclick={()=>moveScene(rows.indexOf(selectedRow),1)}><ArrowDown size={14}/></Button></div><form onsubmit={(event)=>{event.preventDefault();void saveTimeline();}} oninput={()=>dirty=true}><div class="timeline-editor-top"><div><label for="timeline-title">Scene title</label><Input id="timeline-title" bind:value={selectedRow.scene.title} maxlength={180} required disabled={saving}/></div><div><label for="timeline-duration">Duration (s)</label><Input id="timeline-duration" type="number" min={0} max={3600} step="0.1" bind:value={selectedRow.scene.duration} disabled={saving}/></div></div><label for="timeline-prompt">Scene direction</label><Textarea id="timeline-prompt" bind:value={selectedRow.scene.prompt} maxlength={8000} rows={3} disabled={saving}/><h3 class="timeline-shot-heading">Shots in this scene</h3>{#each selectedRow.shots as shot,index}<details class="timeline-shot" open={selectedShot===shot.id} ontoggle={(event)=>{if(event.currentTarget.open)selectedShot=shot.id;else if(selectedShot===shot.id)selectedShot='';}}><summary><span>SHOT {String(index+1).padStart(2,'0')}</span><strong>{shot.title}</strong><small>{shot.duration}s</small></summary><div class="timeline-shot-actions"><Button variant="outline" size="sm" type="button" disabled={index===0||saving} onclick={()=>moveShot(selectedRow,index,-1)}><ArrowUp size={13}/>Earlier</Button><Button variant="outline" size="sm" type="button" disabled={index===selectedRow.shots.length-1||saving} onclick={()=>moveShot(selectedRow,index,1)}><ArrowDown size={13}/>Later</Button></div><div class="timeline-editor-top"><div><label for={`shot-title-${shot.id}`}>Shot title</label><Input id={`shot-title-${shot.id}`} bind:value={shot.title} maxlength={180} required disabled={saving}/></div><div><label for={`shot-duration-${shot.id}`}>Duration (s)</label><Input id={`shot-duration-${shot.id}`} type="number" min={0} max={3600} step="0.1" bind:value={shot.duration} disabled={saving}/></div></div><label for={`shot-prompt-${shot.id}`}>Shot direction</label><Textarea id={`shot-prompt-${shot.id}`} bind:value={shot.prompt} maxlength={8000} rows={3} disabled={saving}/></details>{:else}<p class="field-help">No shots yet. Add them in the Shots tab.</p>{/each}<div class="timeline-save-state"><span>{dirty?'Unsaved storyboard changes':'Storyboard is up to date'}</span><Button type="submit" size="sm" disabled={saving||!dirty}><Save size={14}/>Save changes</Button></div></form></div>{/if}</div>
    {/if}</section>
  {/if}
</div>
<style>
.scene-order-actions{display:flex;align-items:center;gap:4px;margin-bottom:16px;font-size:12px;color:var(--muted-foreground)}.scene-order-actions>span{margin-right:auto}

.generation-disclosure{border-bottom:1px solid var(--border);padding-bottom:16px}.generation-disclosure>summary{cursor:pointer;padding:12px 0;font-size:14px;font-weight:600}.generation-disclosure>summary>span{font-size:12px;color:var(--muted-foreground);font-weight:400;margin-left:12px}.storyboard-surface{display:grid;grid-template-columns:minmax(0,1fr) minmax(280px,340px);gap:16px;align-items:start}.storyboard-surface :global(.story-graph){margin:0}.timeline-empty :global(a){margin-top:20px}.storyboard-timeline{margin-top:24px!important;padding-top:24px!important}.timeline-scene-editor{padding:18px!important}@media(max-width:1000px){.storyboard-surface{grid-template-columns:1fr}.generation-disclosure>summary>span{display:block;margin:6px 0 0}}

.workflow-heading{display:flex;gap:35px;align-items:start;justify-content:space-between;margin:8px 0 27px}.workflow-heading>div:first-child{max-width:520px}.workflow-heading h2{font-size:20px!important}.workflow-heading p{font-size:12px;color:var(--muted-foreground);line-height:1.9;margin-top:12px}.workflow-balance{display:flex;gap:10px;align-items:center;flex-shrink:0;background:var(--muted);border-radius:7px;padding:14px 17px;color:var(--muted-foreground);font-size:12px}.workflow-balance strong{color:var(--foreground);font-size:16px;font-weight:550}.workflow-balance small{display:block;font-size:12px;margin-top:2px}.workflow-compose{padding:25px 27px;background:var(--card);border:1px solid var(--border);border-radius:12px;margin-top:18px}.workflow-compose label:first-child{margin-top:0}.workflow-options{display:grid;grid-template-columns:1fr 1fr 1fr 1.1fr;gap:20px;margin-top:25px;align-items:end}.workflow-length{padding:8px 0 8px 20px;border-left:1px solid var(--border)}.workflow-length span{font-size:12px;color:var(--muted-foreground);display:block}.workflow-length strong{display:block;font-size:15px;font-weight:550;color:var(--foreground);margin-top:3px}.workflow-footer{border-top:1px solid var(--border);margin-top:25px;padding-top:22px;display:flex;justify-content:space-between;align-items:center;gap:20px}.workflow-footer>span{display:flex;gap:7px;align-items:center;font-size:12px;color:var(--muted-foreground)}.workflow-quote{padding:25px;background:var(--accent);border-radius:8px;margin-top:25px;display:flex;align-items:center;justify-content:space-between;gap:30px}.workflow-quote .eyebrow{font-size:12px;margin-bottom:6px}.workflow-quote strong{font-size:28px;letter-spacing:-.7px;font-weight:550;color:var(--foreground)}.workflow-quote p{font-size:12px;color:var(--muted-foreground);line-height:1.9;max-width:440px;margin-top:5px}.assembly-empty{display:flex;gap:15px;padding:30px 0;align-items:center;color:var(--muted-foreground)}.assembly-empty p{font-size:12px;max-width:460px;line-height:1.8}.assembly-order{list-style:none;padding:0;margin:20px 0 0}.assembly-order li{display:flex;gap:14px;align-items:center;padding:12px 0;border-top:1px solid var(--border)}.assembly-order li>span{font-size:12px;color:var(--muted-foreground)}.assembly-order li>strong{font-size:12px;flex:1;font-weight:550;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:600px}.assembly-order li>div{display:flex;gap:4px}.storyboard-timeline{margin-top:45px;border-top:1px solid var(--border);padding-top:35px}.storyboard-timeline>.section-heading{gap:25px;align-items:start}.storyboard-timeline h2{font-size:20px!important}.storyboard-timeline .section-heading p{margin-top:9px}.timeline-toolbar{display:flex;gap:8px;flex-shrink:0}.timeline-disclosure{font-size:12px;color:var(--muted-foreground);max-width:720px;line-height:1.9;margin:0 0 25px!important}.timeline-empty{text-align:center;padding:45px 25px;border:1px dashed var(--border);border-radius:12px;color:var(--muted-foreground)}.timeline-empty :global(svg){margin:0 auto 15px}.timeline-empty h3{font-size:14px}.timeline-empty p{font-size:12px;max-width:440px;margin:10px auto 0;line-height:1.9}.timeline-scene-editor{border:1px solid var(--border);background:var(--card);border-radius:12px;padding:24px}.timeline-editor-top{display:grid;grid-template-columns:minmax(0,1fr) 100px;gap:17px}.timeline-editor-top label{margin-top:0}.timeline-shot-heading{font-size:12px;margin:25px 0 15px!important}.timeline-shot{border-top:1px solid var(--border);padding:17px 0}.timeline-shot summary{display:flex;align-items:center;gap:12px;cursor:pointer}.timeline-shot summary>span{font-size:12px;letter-spacing:.6px;color:var(--muted-foreground);flex-shrink:0}.timeline-shot summary>strong{font-size:12px;font-weight:550;flex:1}.timeline-shot summary>small{font-size:12px;color:var(--muted-foreground)}.timeline-shot-actions{display:flex;justify-content:end;gap:8px;margin:17px 0}.timeline-save-state{border-top:1px solid var(--border);padding-top:20px;margin-top:20px;display:flex;gap:10px;align-items:center;justify-content:space-between}.timeline-save-state>span{font-size:12px;color:var(--muted-foreground)}@media(max-width:1000px){.workflow-heading{gap:20px}.workflow-balance{padding:11px}.workflow-options{gap:13px}.workflow-length{padding-left:13px}.storyboard-timeline>.section-heading{flex-direction:column;gap:15px}}@media(max-width:650px){.workflow-heading{flex-direction:column;gap:20px}.workflow-heading h2{font-size:20px!important}.workflow-balance{width:100%}.workflow-options{grid-template-columns:1fr 1fr}.workflow-length{border:0;padding-left:0}.workflow-compose{padding:21px 18px}.workflow-footer,.workflow-quote{flex-direction:column;align-items:start;gap:17px}.workflow-footer :global(button),.workflow-quote :global(button){width:100%}.assembly-order li>strong{max-width:140px;font-size:12px}.assembly-order li{gap:8px}.assembly-order li>div{gap:0}.timeline-scene-editor{padding:19px}.timeline-editor-top{grid-template-columns:minmax(0,1fr) 80px;gap:11px}.timeline-save-state{flex-direction:column;align-items:start}.timeline-save-state :global(button){width:100%}.timeline-toolbar{width:100%;justify-content:space-between}.workflow-compose .field-help{font-size:12px!important}}
</style>
