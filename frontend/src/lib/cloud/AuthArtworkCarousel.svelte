<script lang="ts">
 import {onMount} from 'svelte';
 import {ChevronLeft,ChevronRight,Pause,Play} from '@lucide/svelte';
 import {Button} from '$lib/components/ui/button';
 const slides=[
  {src:'/brand/auth/desert.webp',title:'An idea can take you anywhere.',caption:'Create your next world with 6eek.',alt:'A cloaked traveller crosses dark dunes beneath a giant golden sun'},
  {src:'/brand/auth/pavilion.webp',title:'Find a different point of view.',caption:'Discover stories worth staying for.',alt:'A lantern-lit pavilion reflected in a misty lake among floating mountain peaks'},
  {src:'/brand/auth/motion.webp',title:'Make something worth sharing.',caption:'Your story. A new audience. 6eek.',alt:'A skateboarder in an orange jacket leaps above a rain-lit futuristic street'}
 ];
 let current=$state(0),paused=$state(false),reduced=$state(false),visible=$state(false),inView=$state(false),desktop=$state(false),interacting=$state(false),panel:HTMLElement;
 const autoplay=$derived(!paused&&!reduced&&visible&&inView&&desktop&&!interacting);
 function show(index:number){current=(index+slides.length)%slides.length;}
 onMount(()=>{
  const motion=matchMedia('(prefers-reduced-motion: reduce)'),screen=matchMedia('(min-width: 1024px)');
  const sync=()=>{reduced=motion.matches;desktop=screen.matches;visible=document.visibilityState==='visible';};sync();
  motion.addEventListener('change',sync);screen.addEventListener('change',sync);document.addEventListener('visibilitychange',sync);
  const observer=new IntersectionObserver(([entry])=>inView=entry.isIntersecting);observer.observe(panel);
  return()=>{motion.removeEventListener('change',sync);screen.removeEventListener('change',sync);document.removeEventListener('visibilitychange',sync);observer.disconnect();};
 });
 $effect(()=>{if(!autoplay)return;const timer=setInterval(()=>show(current+1),7000);return()=>clearInterval(timer);});
</script>
<section bind:this={panel} class="artwork-carousel" aria-label="6eek creative inspiration" aria-roledescription="carousel" onmouseenter={()=>interacting=true} onmouseleave={()=>interacting=false} onfocusin={()=>interacting=true} onfocusout={(e)=>{if(!e.currentTarget.contains(e.relatedTarget as Node|null))interacting=false;}}>
 <div class="artwork-stage">
  {#each slides as slide,index}<div class="artwork-slide" class:current={current===index} aria-hidden={current!==index} role="group" aria-roledescription="slide" aria-label={`${index+1} of ${slides.length}`}>
   {#if desktop&&(index===0||current===index)}<img src={slide.src} alt={slide.alt} width="1122" height="1402" loading={index===0?'eager':'lazy'} fetchpriority={index===0?'high':'auto'} decoding="async"/>{/if}
  </div>{/each}
 </div>
 <div class="artwork-shade"></div>
 <div class="artwork-content"><div aria-live={autoplay?'off':'polite'} aria-atomic="true"><h2>{slides[current].title}</h2><p>{slides[current].caption}</p></div>
  <div class="artwork-controls"><div class="artwork-dots" role="group" aria-label="Choose artwork">{#each slides as slide,index}<button type="button" aria-label={`Show artwork ${index+1}: ${slide.title}`} aria-pressed={current===index} class:active={current===index} onclick={()=>show(index)}><span></span></button>{/each}</div><div class="artwork-actions"><Button type="button" variant="ghost" size="icon" aria-label="Previous artwork" onclick={()=>show(current-1)}><ChevronLeft size={18}/></Button><Button type="button" variant="ghost" size="icon" aria-label={paused?'Play artwork carousel':'Pause artwork carousel'} aria-pressed={paused} onclick={()=>paused=!paused}>{#if paused}<Play size={16}/>{:else}<Pause size={16}/>{/if}</Button><Button type="button" variant="ghost" size="icon" aria-label="Next artwork" onclick={()=>show(current+1)}><ChevronRight size={18}/></Button></div></div>
 </div>
</section>
<style>.artwork-carousel{position:relative;min-width:0;height:100%;min-height:100svh;overflow:hidden;isolation:isolate;background:#1b1c1a;color:#ebe9e2}.artwork-stage,.artwork-slide,.artwork-shade{position:absolute;inset:0}.artwork-slide{opacity:0;transition:opacity 800ms ease}.artwork-slide.current{opacity:1}.artwork-slide img{width:100%;height:100%;object-fit:cover;display:block}.artwork-shade{background:linear-gradient(180deg,transparent 35%,rgb(11 12 11 / .28) 62%,rgb(11 12 11 / .87) 100%);pointer-events:none}.artwork-content{position:absolute;inset:auto 0 0;padding:clamp(32px,4.5vw,72px);z-index:1}.artwork-content h2{font-size:clamp(28px,3.2vw,48px);font-weight:500;letter-spacing:-.035em;line-height:1.14;max-width:12ch;text-wrap:balance;margin:0 0 18px}.artwork-content p{font-size:14px;line-height:1.6;color:rgb(235 233 226 / .82);margin:0}.artwork-controls{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-top:32px}.artwork-dots,.artwork-actions{display:flex;align-items:center;gap:4px}.artwork-dots button{width:32px;height:36px;display:grid;place-items:center;background:none;border:0;cursor:pointer}.artwork-dots span{display:block;width:6px;height:6px;border-radius:50%;background:rgb(235 233 226 / .42);transition:background 200ms,width 200ms}.artwork-dots .active span{width:22px;border-radius:4px;background:#ff5b1f}.artwork-actions :global(button){color:#ebe9e2;border-radius:50%;background:rgb(11 12 11 / .2)}.artwork-actions :global(button:hover){background:rgb(235 233 226 / .16);color:white}.artwork-dots button:focus-visible,.artwork-actions :global(button:focus-visible){outline:2px solid #ff5b1f;outline-offset:3px}@media(max-width:1023px){.artwork-carousel{display:none}}@media(prefers-reduced-motion:reduce){.artwork-slide,.artwork-dots span{transition:none}}</style>
