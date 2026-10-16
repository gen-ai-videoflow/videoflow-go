<script lang="ts">
  import { tick } from 'svelte';
  import { SvelteFlow, Background, Controls, MarkerType, useSvelteFlow, type Node, type Edge } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';
  import { Button } from '$lib/components/ui/button';
  import { Network, ArrowDownRight, LoaderCircle } from '@lucide/svelte';
  import StoryboardNode from './StoryboardNode.svelte';
  import FlowHelper from '$lib/canvas/FlowHelper.svelte';
  interface Entity {id:string;title:string;duration:number;}
  interface SceneRow {scene:Entity;shots:Entity[];}
  let {rows,selectedScene='',selectedShot='',loading=false,onselect}: {rows:SceneRow[];selectedScene?:string;selectedShot?:string;loading?:boolean;onselect:(selection:{sceneId:string;shotId?:string})=>void}=$props();
  let flow=$state.raw<ReturnType<typeof useSvelteFlow>>();
  const nodeTypes={storyboard:StoryboardNode};
  const computedNodes=$derived.by(()=>{
    const result:Node[]=[];
    for(const [sceneIndex,row]of rows.entries()){
      const x=sceneIndex*330;
      result.push({id:`scene:${row.scene.id}`,type:'storyboard',position:{x,y:0},selected:selectedScene===row.scene.id&&!selectedShot,data:{kind:'scene',title:row.scene.title,number:sceneIndex+1,duration:row.shots.length?row.shots.reduce((total,shot)=>total+(Number(shot.duration)||0),0):row.scene.duration,shots:row.shots.length,onfocus:()=>flow?.fitView({nodes:[{id:`scene:${row.scene.id}`}],minZoom:.6,maxZoom:1,duration:0}),onactivate:()=>onselect({sceneId:row.scene.id})}});
      for(const [shotIndex,shot]of row.shots.entries())result.push({id:`shot:${shot.id}`,type:'storyboard',position:{x,y:210+shotIndex*185},selected:selectedShot===shot.id,data:{kind:'shot',title:shot.title,number:shotIndex+1,duration:shot.duration,onfocus:()=>flow?.fitView({nodes:[{id:`shot:${shot.id}`}],minZoom:.6,maxZoom:1,duration:0}),onactivate:()=>onselect({sceneId:row.scene.id,shotId:shot.id})}});
    }
    return result;
  });
  let nodes=$state.raw<Node[]>([]);
  $effect(()=>{nodes=computedNodes;});
  const edges=$derived.by(()=>{
    const result:Edge[]=[];
    for(const [index,row]of rows.entries()){
      if(index<rows.length-1)result.push({id:`next-scene:${row.scene.id}`,source:`scene:${row.scene.id}`,target:`scene:${rows[index+1].scene.id}`,sourceHandle:'next',targetHandle:'previous',type:'smoothstep',label:'Next scene',labelStyle:'font-size:12px;fill:var(--muted-foreground);',style:'stroke:var(--muted-foreground);stroke-width:2px;',markerEnd:{type:MarkerType.ArrowClosed,color:'var(--muted-foreground)'}});
      for(const [shotIndex,shot]of row.shots.entries())result.push({id:`shot-order:${shot.id}`,source:shotIndex===0?`scene:${row.scene.id}`:`shot:${row.shots[shotIndex-1].id}`,target:`shot:${shot.id}`,sourceHandle:'shots',targetHandle:'parent',type:'smoothstep',label:shotIndex===0?'Scene shots':undefined,labelStyle:'font-size:12px;fill:var(--muted-foreground);',style:'stroke:var(--muted-foreground);stroke-width:1.5px;',markerEnd:{type:MarkerType.ArrowClosed,color:'var(--muted-foreground)'}});
    }
    return result;
  });
  // Refit after structural changes, while preserving the viewport during editing.
  $effect(()=>{
    const structure=rows.map(row=>`${row.scene.id}:${row.shots.map(shot=>shot.id).join(',')}`).join('|');
    const instance=flow;
    if(instance&&structure)void tick().then(()=>instance.fitView({padding:.22,maxZoom:1,duration:0}));
  });
</script>
<section class="story-graph" aria-label="Scene and shot flow graph">
  <div class="graph-heading"><div><h3><Network size={17}/>Story flow</h3><p>Select a scene or shot to edit its direction.</p></div>{#if rows.length}<Button size="sm" variant="outline" onclick={()=>{document.getElementById('storyboard-editor')?.scrollIntoView({behavior:'auto',block:'start'});document.getElementById(selectedShot ? `shot-title-${selectedShot}` : 'timeline-title')?.focus({preventScroll:true});}}>Edit selection <ArrowDownRight size={15}/></Button>{/if}</div>
  {#if loading}<div class="graph-empty" role="status"><LoaderCircle size={24} class="animate-spin"/><p>Loading your story structure…</p></div>{:else if !rows.length}<div class="graph-empty"><Network size={30}/><strong>Your story has room to grow.</strong><p>Generate a storyboard or add scenes and shots to see their connections here.</p></div>{:else}<div class="graph-canvas"><SvelteFlow {nodes} {edges} {nodeTypes} nodesDraggable={false} nodesConnectable={false} nodesFocusable={false} edgesFocusable={false} elementsSelectable={false} deleteKey={null} selectionKey={null} zoomOnScroll={false} preventScrolling={false} fitView fitViewOptions={{padding:.22,maxZoom:1}} minZoom={.18} maxZoom={1.5} colorMode="system" proOptions={{hideAttribution:true}} aria-label="Read-only story structure. Tab to a scene or shot and press Enter to select it."><FlowHelper register={(instance)=>flow=instance}/><Background gap={24} size={1} patternColor="var(--border)"/><Controls showLock={false} position="bottom-left" buttonBgColor="var(--card)" buttonColor="var(--foreground)" buttonBorderColor="var(--border)" fitViewOptions={{padding:.22,maxZoom:1}} aria-label="Story graph zoom and fit controls"/></SvelteFlow></div><p class="graph-help">Pan to explore · Pinch or use controls to zoom · Select to edit</p>{/if}
</section>
<style>
.story-graph{border:1px solid var(--border);border-radius:12px;overflow:hidden;background:var(--muted);margin:26px 0}.graph-heading{display:flex;align-items:center;justify-content:space-between;gap:25px;padding:21px 23px;border-bottom:1px solid var(--border);background:var(--card)}.graph-heading h3{display:flex;gap:9px;align-items:center;font-size:15px;font-weight:550}.graph-heading p{font-size:12px;color:var(--muted-foreground);line-height:1.8;margin-top:7px;max-width:570px}.graph-canvas{height:440px}.graph-help{padding:14px 23px;font-size:12px;color:var(--muted-foreground);line-height:1.8;border-top:1px solid var(--border);background:var(--card)}.graph-empty{min-height:240px;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:15px;padding:35px;color:var(--muted-foreground);text-align:center}.graph-empty strong{font-size:14px;font-weight:550}.graph-empty p{font-size:12px;line-height:1.8;max-width:390px}.graph-canvas :global(.svelte-flow__node-storyboard){background:transparent;border:0;border-radius:10px;padding:0;box-shadow:none;width:240px}.graph-canvas :global(.svelte-flow__controls){box-shadow:none;border:1px solid var(--border);border-radius:5px;overflow:hidden}.graph-canvas :global(.svelte-flow__controls-button){width:36px;height:36px}.graph-canvas :global(.svelte-flow__controls-button:focus-visible){outline:2px solid #ba4027;outline-offset:-3px}@media(max-width:800px){.graph-heading{align-items:start;flex-direction:column;gap:16px}.graph-canvas{height:360px}.graph-heading,.graph-help{padding:18px}.graph-heading p{font-size:12px}}@media(prefers-reduced-motion:reduce){.graph-canvas :global(*){transition:none!important;animation:none!important}}
</style>
