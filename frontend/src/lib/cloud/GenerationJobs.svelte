<script lang="ts">
  import { onMount } from 'svelte';
  import { Button } from '$lib/components/ui/button';
  import { Clapperboard, ArrowDownToLine, ChevronDown, ChevronUp, LoaderCircle, Square, RefreshCw, FileText } from '@lucide/svelte';
  import { request } from './api';
  import type { PipelineProgress } from './generated';
  let {jobs, onrefresh, focusId=''}: {jobs:any[];onrefresh:()=>Promise<void>;focusId?:string}=$props();
  let busy=$state('');let error=$state('');let notice=$state('');
  let expanded=$state('');let detail=$state<(PipelineProgress & {error?:string})|null>(null);let detailLoading=$state(false);let detailError=$state('');
  let lastFocus='';
  $effect(()=>{if(focusId&&focusId!==lastFocus){lastFocus=focusId;expanded=focusId;detail=null;void loadDetail(focusId);}});
  const active=(job:any)=>['queued','running','processing','submitted','planning','generating','assembling'].includes(job.status);
  const pipeline=(job:any)=>['pipeline','storyboard','assembly'].includes(job.kind || job.operation || job.type);
  const label=(value:string)=>value ? value.replaceAll('_',' ').replace(/\b\w/g,char=>char.toUpperCase()) : 'Awaiting update';
  const date=(value:string)=>value ? new Date(value).toLocaleString(undefined,{month:'short',day:'numeric',hour:'numeric',minute:'2-digit'}) : '';
  async function refresh(){try{await onrefresh();if(expanded)await loadDetail(expanded,false);}catch(e){error=(e as Error).message;}}
  async function loadDetail(id:string,showLoading=true){
    if(showLoading)detailLoading=true;detailError='';
    try{const result=await request(`/jobs/${encodeURIComponent(id)}/pipeline`);if(expanded===id)detail=result;}
    catch(e){if(expanded===id)detailError=(e as Error).message;}
    finally{if(expanded===id)detailLoading=false;}
  }
  function toggle(job:any){if(expanded===job.id){expanded='';detail=null;return;}expanded=job.id;detail=null;void loadDetail(job.id);}
  async function cancel(job:any){
    busy=job.id;error='';notice='';
    try{await request(`/jobs/${encodeURIComponent(job.id)}/cancel`,{});await onrefresh();notice='Cancellation accepted.';}
    catch(e){error=(e as Error).message;}finally{busy='';}
  }
  async function download(job:any){
    busy=job.id;error='';
    try{const result=await request(`/jobs/${encodeURIComponent(job.id)}/download`);if(!result.url)throw new Error('This output is not ready to download.');window.open(result.url,'_blank','noopener,noreferrer');}
    catch(e){error=(e as Error).message;}finally{busy='';}
  }
  onMount(()=>{const timer=setInterval(()=>{if(jobs.some(active))void refresh();},10000);return()=>clearInterval(timer);});
</script>
<section class="generation-jobs">
  <div class="section-heading"><div><h2>Generation activity</h2><p class="subtle">Saved progress. Updates automatically while work is in progress.</p></div><Button size="sm" variant="ghost" onclick={refresh}><RefreshCw size={14}/>Refresh</Button></div>
  {#if error}<div class="cloud-alert" role="alert">{error}</div>{/if}{#if notice}<div class="cloud-notice" role="status">{notice}</div>{/if}
  {#if jobs.length}<div class="generation-list">{#each jobs as job (job.id)}<article class="generation-row"><div class="generation-summary"><div class="generation-icon">{#if active(job)&&job.status!=='queued'}<LoaderCircle size={19} class="animate-spin"/>{:else}<Clapperboard size={19}/>{/if}</div><div class="generation-name"><h3>{job.title || job.prompt || job.name || `${pipeline(job) ? 'Project video' : 'Video generation'} · ${job.id.slice(0,8)}`}</h3><p>{date(job.created_at)}{#if typeof job.credits==='number'}<span>Quoted {job.credits.toLocaleString()} credits</span>{/if}</p></div><span class="status" class:job-failed={job.status==='failed'}>{label(job.stage || job.status)}</span><div class="generation-actions">{#if pipeline(job)}<Button size="sm" variant="ghost" aria-expanded={expanded===job.id} onclick={()=>toggle(job)}>{expanded===job.id ? 'Hide stages' : 'View stages'}{#if expanded===job.id}<ChevronUp size={14}/>{:else}<ChevronDown size={14}/>{/if}</Button>{/if}{#if job.status==='queued'||job.can_cancel}<Button size="sm" variant="outline" disabled={busy===job.id} onclick={()=>cancel(job)}><Square size={12}/>Cancel</Button>{/if}{#if ['completed','succeeded','complete'].includes(job.status)}<Button size="sm" variant="outline" disabled={busy===job.id} onclick={()=>download(job)}><ArrowDownToLine size={14}/>Download</Button>{/if}</div></div>{#if job.error}<p class="generation-error" role="status">{job.error}</p>{/if}
  {#if expanded===job.id}<div class="pipeline-details">{#if detailLoading}<p class="detail-loading"><LoaderCircle size={15} class="animate-spin"/>Loading saved stages…</p>{:else if detailError}<div class="cloud-alert" role="alert">{detailError}</div>{:else if detail}<div class="pipeline-stage"><span class="eyebrow">CURRENT STAGE</span><strong>{label(detail.stage || detail.status)}</strong></div>{#if detail.error}<p class="generation-error">{detail.error}</p>{/if}{#if detail.script}<details class="pipeline-script"><summary><FileText size={15}/>Generated screenplay</summary><p>{detail.script}</p></details>{/if}<div class="pipeline-scenes">{#each detail.scenes || [] as scene,index}<div><span class="scene-index">{String(index+1).padStart(2,'0')}</span><div><h4>{scene.title || `Scene ${index+1}`}</h4><p>{scene.prompt}</p><small>{scene.duration ? `${scene.duration} seconds` : ''}</small></div><span class="status">{label(scene.status)}</span></div>{/each}</div>{/if}</div>{/if}</article>{/each}</div>{:else}<p class="quiet-empty">Your video jobs will appear here. Leaving this page does not interrupt processing.</p>{/if}
</section>
<style>
.generation-jobs{margin-top:35px}.generation-jobs .section-heading p{margin-top:5px}.generation-row{border-bottom:1px solid var(--border);padding:21px 0}.generation-summary{display:flex;gap:15px;align-items:center}.generation-icon{color:var(--muted-foreground);flex-shrink:0}.generation-name{flex:1;min-width:0}.generation-name h3{font-size:12px;font-weight:550;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:620px}.generation-name p{display:flex;gap:15px;font-size:12px;color:var(--muted-foreground);margin-top:5px;flex-wrap:wrap}.generation-actions{display:flex;align-items:center;gap:8px}.generation-error{font-size:12px;color:var(--muted-foreground);background:var(--muted);border-radius:5px;padding:10px 14px;margin:15px 0 0!important}.job-failed{background:var(--muted);color:var(--muted-foreground)}.pipeline-details{margin:20px 0 0 34px;border-left:2px solid var(--border);padding:3px 0 0 23px}.detail-loading{display:flex;gap:10px;align-items:center;color:var(--muted-foreground);font-size:12px}.pipeline-stage .eyebrow{font-size:12px;margin-bottom:4px}.pipeline-stage>strong{font-weight:550;font-size:13px}.pipeline-script{margin:20px 0;border-top:1px solid var(--border);padding-top:15px}.pipeline-script summary{display:flex;align-items:center;gap:7px;font-size:12px;font-weight:550;cursor:pointer}.pipeline-script>p{white-space:pre-wrap;font-size:12px;color:var(--muted-foreground);line-height:1.9;margin-top:15px;max-height:400px;overflow:auto}.pipeline-scenes{display:grid;margin-top:15px}.pipeline-scenes>div{display:flex;gap:13px;align-items:start;padding:15px 0;border-top:1px solid var(--border)}.scene-index{font-size:12px;color:var(--muted-foreground);margin-top:3px}.pipeline-scenes>div>div{flex:1;min-width:0}.pipeline-scenes h4{font-size:12px;font-weight:550}.pipeline-scenes p{font-size:12px;color:var(--muted-foreground);line-height:1.8;margin:6px 0}.pipeline-scenes small{font-size:12px;color:var(--muted-foreground)}@media(max-width:1000px){.generation-summary{flex-wrap:wrap}.generation-name{flex-basis:60%}.generation-actions{margin-left:34px;width:100%}.pipeline-details{margin-left:5px;padding-left:17px}}@media(max-width:520px){.generation-summary{gap:10px}.generation-name h3{max-width:220px;font-size:12px}.generation-name p{gap:5px}.generation-summary>.status{margin-left:auto}.generation-actions{margin-left:29px;gap:5px}.pipeline-scenes>div{flex-wrap:wrap}.pipeline-scenes>div>.status{margin-left:25px}.pipeline-scenes>div>div{flex-basis:80%}}
</style>
