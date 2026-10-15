<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte';
  import { Clapperboard, Film } from '@lucide/svelte';
  let {data,selected}:NodeProps=$props();
  const scene=$derived(data.kind==='scene');
</script>
<Handle type="target" position={Position.Left} id="previous" isConnectable={false} style="opacity:0"/>
<Handle type="target" position={Position.Top} id="parent" isConnectable={false} style="opacity:0"/>
<button class="story-node nodrag nopan" class:scene class:selected aria-pressed={!!selected} aria-label={`${scene?'Scene':'Shot'} ${data.number}: ${data.title}, ${data.duration} seconds. Select to edit.`} onfocus={(event)=>{if(event.currentTarget.matches(':focus-visible')&&typeof data.onfocus==='function')data.onfocus();}} onclick={()=>{if(typeof data.onactivate==='function')data.onactivate();}}>
  <span class="story-node-kind">{#if scene}<Clapperboard size={15}/>{:else}<Film size={15}/>{/if}{scene?'SCENE':'SHOT'} {String(data.number).padStart(2,'0')}</span>
  <strong>{String(data.title)}</strong>
  <span class="story-node-time">{Number(data.duration)||0}s{#if scene} · {Number(data.shots)||0} shots{/if}</span>
</button>
<Handle type="source" position={Position.Right} id="next" isConnectable={false} style="opacity:0"/>
<Handle type="source" position={Position.Bottom} id="shots" isConnectable={false} style="opacity:0"/>
<style>
.story-node{width:240px;min-height:122px;padding:17px 19px;border:1px solid #bac6aa;border-radius:10px;background:#fff;text-align:left;cursor:pointer;display:flex;flex-direction:column;gap:11px;color:#293324;transition:border-color .15s,background .15s}.story-node.scene{background:#eef3e5;border-color:#a8ba92}.story-node.selected{border:2px solid #60794b;padding:16px 18px;background:#e5eed7}.story-node:focus-visible{outline:3px solid #ba4027;outline-offset:4px}.story-node-kind{display:flex;align-items:center;gap:8px;color:#596b49;font-size:12px;letter-spacing:.5px;font-weight:550}.story-node strong{font-size:14px;line-height:1.5;font-weight:550;display:-webkit-box;-webkit-line-clamp:2;line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;overflow-wrap:anywhere}.story-node-time{font-size:12px;color:#5e6c52}
</style>
