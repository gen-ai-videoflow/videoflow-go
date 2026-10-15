<script lang="ts">
  import { onMount } from 'svelte';
  import { request } from './api';

  let {videoId,onerror,active=true,muted=false,autoplay=false,resumeAt=0,signedIn=false,captions=false,dataSaving=false,pauseWhenHidden=false,onended=()=>{},onMutedChange=()=>{}}:{videoId:string;onerror:(message:string)=>void;active?:boolean;muted?:boolean;autoplay?:boolean;resumeAt?:number;signedIn?:boolean;captions?:boolean;dataSaving?:boolean;pauseWhenHidden?:boolean;onended?:()=>void;onMutedChange?:(muted:boolean)=>void}=$props();
  let element=$state<HTMLVideoElement>();
  $effect(()=>{if(element)element.muted=muted;});
  $effect(()=>{if(element){if(!active)element.pause();else if(autoplay&&element.readyState>=2)void element.play().catch(()=>{});}});
  let captionsUrl=$state('');

  onMount(() => {
    if(!element)return;
    const player=element;
    let hls:import('hls.js').default | undefined;
    let stopped = false;
    let expiresAt = 0;
    let grantRequest:Promise<any> | undefined;
    let renewingForResume = false;
    let sessionId = '';
    let counting = false;
    let visible = !document.hidden;
    let lastClock = performance.now();
    let lastMediaTime = 0;
    let pendingSeconds = 0;
    let lastHeartbeat = performance.now();

    // Count only time that advances during visible, uninterrupted playback.
    // Comparing wall and media clocks also excludes seeking and playback-rate inflation.
    function account() {
      const now = performance.now();
      const mediaTime = player.currentTime;
      if (counting) pendingSeconds += Math.max(0,Math.min((now-lastClock)/1000,mediaTime-lastMediaTime));
      lastClock = now;
      lastMediaTime = mediaTime;
    }
    function flush() {
      account();
      if(signedIn&&player.currentTime>0)void request(`/videos/${videoId}/progress`,{position_ms:Math.floor(player.currentTime*1000)},'PUT',{},true).catch(()=>{});
      const seconds = Math.min(15,Math.floor(pendingSeconds));
      if (!sessionId || seconds < 1) return;
      pendingSeconds -= seconds;
      void request('/watch-events',{session_id:sessionId,event_id:crypto.randomUUID(),seconds},'POST',{},true).catch(()=>{});
    }
    function stopCounting(send=true) {
      account();
      counting = false;
      if (send) flush();
    }
    function startCounting() {
      account();
      counting = active && visible && !player.paused && !player.seeking && player.readyState >= 3;
    }
    async function grant() {
      if (grantRequest) return grantRequest;
      grantRequest = (async()=>{
        const result = await request(`/videos/${videoId}/playback-grant`,{});
        if (stopped) return result;
        if (result.authorize_url) {
          const response = await fetch(result.authorize_url,{credentials:'include'});
          if (!response.ok) throw new Error('Playback authorization failed. Please try again.');
        }
        captionsUrl=result.captions_url || '';
        expiresAt = result.expires_at ? Date.parse(result.expires_at) : Date.now() + (result.expires_in || 300)*1000;
        return result;
      })();
      try {return await grantRequest;} finally {grantRequest=undefined;}
    }
    async function onPlay() {
      if (renewingForResume || stopped) return;
      if (Date.now() < expiresAt - 30000) {hls?.startLoad();return;}
      // Do not let HLS retry old cookies after a long pause.
      renewingForResume=true;
      stopCounting();
      player.pause();
      hls?.stopLoad();
      try {
        await grant();
        if (!stopped) {hls?.startLoad(); await player.play().catch(()=>{}); startCounting();}
      } catch(error) {if(!stopped)onerror((error as Error).message);}
      finally {renewingForResume=false;}
    }
    function onVisibility() {
      account();
      visible = !document.hidden;
      if (!visible) {stopCounting();if(pauseWhenHidden){player.pause();hls?.stopLoad();}}
      else if (player.readyState >= 3) startCounting();
    }
    const listeners: [string,EventListener][] = [
      ['play',()=>{void onPlay();}],
      ['playing',()=>{if(!renewingForResume)startCounting();}],
      ['pause',()=>stopCounting()],
      ['ended',()=>{stopCounting();onended();}],
      ['waiting',()=>stopCounting()],
      ['stalled',()=>{if(player.readyState<3)stopCounting();}],
      ['seeking',()=>{counting=false;lastClock=performance.now();lastMediaTime=player.currentTime;flush();}],
      ['seeked',()=>{if(player.readyState>=3)startCounting();}]
    ];
    for (const [event,listener] of listeners) player.addEventListener(event,listener);
    document.addEventListener('visibilitychange',onVisibility);
    window.addEventListener('pagehide',flush);

    // Analytics are best effort and never gate playback. Anonymous sessions are
    // intentionally unavailable; their 401 response does not interrupt viewing.
    if(signedIn)void request('/watch-sessions',{video_id:videoId}).then(result=>{
      if (!stopped) {sessionId=result.id || result.session_id;pendingSeconds=0;lastClock=performance.now();lastMediaTime=player.currentTime;}
    }).catch(()=>{});
    void (async()=>{
      try {
        const result=await grant();
        if (stopped) return;
        if (player.canPlayType('application/vnd.apple.mpegurl') || !result.url.includes('.m3u8')) {
          player.src=result.url;
        } else {
          const Hls=(await import('hls.js')).default;
          if (stopped) return;
          if (!Hls.isSupported()) throw new Error('This browser does not support this video format.');
          hls=new Hls({capLevelToPlayerSize:true,maxBufferLength:dataSaving?10:30,startLevel:dataSaving?0:-1,xhrSetup(xhr){xhr.withCredentials=true;}});
          hls.on(Hls.Events.ERROR,(_,event)=>{
            if(event.fatal && !renewingForResume && !stopped)onerror('Video playback was interrupted. Please try again.');
          });
          if(dataSaving)hls.autoLevelCapping=0;
          hls.loadSource(result.url);
          hls.attachMedia(element);
        }
        if(active&&autoplay)await player.play().catch(()=>{});
      } catch(error) {if(!stopped)onerror((error as Error).message);}
    })();
    const timer=setInterval(()=>{
      account();
      const now=performance.now();
      if(now-lastHeartbeat>=15000){flush();lastHeartbeat=now;}
      if(counting && Date.now()>=expiresAt-45000 && !grantRequest) {
        void grant().catch(error=>{if(!stopped)onerror((error as Error).message);});
      }
    },1000);
    return ()=>{
      stopCounting();
      stopped=true;
      clearInterval(timer);
      for(const [event,listener] of listeners)player.removeEventListener(event,listener);
      document.removeEventListener('visibilitychange',onVisibility);
      window.removeEventListener('pagehide',flush);
      hls?.destroy();
    };
  });
</script>
<video bind:this={element} controls playsinline {muted} preload="metadata" onvolumechange={()=>{if(element)onMutedChange(element.muted);}} onloadedmetadata={()=>{if(element&&resumeAt>0&&resumeAt<element.duration*1000)element.currentTime=resumeAt/1000;}} crossorigin="use-credentials" onerror={()=>onerror('Video could not be played. Please try again.')}>{#if captionsUrl}<track kind="captions" src={captionsUrl} srclang="und" label="Captions" default={captions}/>{/if}</video>
