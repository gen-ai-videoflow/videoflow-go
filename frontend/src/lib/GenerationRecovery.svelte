<script lang="ts">
 import {onMount} from 'svelte';
 import {get,post} from '$lib/api';
 import {toast} from 'svelte-sonner';
 let pending:{key:string;created_at:string}[]=$state([]);
 async function refresh(){pending=await get('/generation-requests')}
 onMount(()=>{refresh().catch(()=>{})});
 async function acknowledge(key:string){
  if(!confirm('Check provider activity and saved work first. Unlocking does not cancel or refund a provider job. Starting again may incur another charge. Have you reviewed them?'))return;
  const reason=prompt('Record what you checked and why it is safe to unlock this request (10+ characters):');if(!reason||reason.trim().length<10)return;
  try{await post(`/generation-requests/${encodeURIComponent(key)}/acknowledge`,{reason,reviewed_provider_and_saved_work:true});await refresh();toast.success('Request reviewed. No generation was started.')}catch(e:any){toast.error(e.message)}
 }
</script>
{#if pending.length}
<section class="mx-auto my-6 max-w-4xl rounded-lg border border-amber-500 p-4 text-sm">
 <h2 class="font-semibold">Interrupted generation responses</h2><p class="mt-2">These requests have no confirmed response. Review Activity, saved work and your provider account before unlocking them. For an uncertain video submission, resume its existing provider job from My videos.</p>
 {#each pending as request(request.key)}<div class="mt-3 flex items-center justify-between gap-3"><span>{request.created_at}</span><button class="rounded border px-3 py-1" onclick={()=>acknowledge(request.key)}>Review and unlock</button></div>{/each}
</section>
{/if}
