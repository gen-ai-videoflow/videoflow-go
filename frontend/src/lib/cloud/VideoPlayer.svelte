<script lang="ts">
  import { onMount } from 'svelte';
  import { request } from './api';

  let {videoId, onerror}: {videoId:string;onerror:(message:string)=>void} = $props();
  let element:HTMLVideoElement;
  let captionsUrl=$state('');

  onMount(() => {
    let hls:import('hls.js').default | undefined;
    let stopped = false;
    let expiresAt = 0;
    let grantRequest:Promise<any> | undefined;
    let renewingForResume = false;
    let sessionId = '';
    let active = false;
    let visible = !document.hidden;
    let lastClock = performance.now();
    let lastMediaTime = 0;
    let pendingSeconds = 0;
    let lastHeartbeat = performance.now();

    // Count only time that advances during visible, uninterrupted playback.
    // Comparing wall and media clocks also excludes seeking and playback-rate inflation.
    function account() {
      const now = performance.now();
      const mediaTime = element.currentTime;
      if (active) pendingSeconds += Math.max(0,Math.min((now-lastClock)/1000,mediaTime-lastMediaTime));
      lastClock = now;
      lastMediaTime = mediaTime;
    }
    function flush() {
      account();
      const seconds = Math.min(15,Math.floor(pendingSeconds));
      if (!sessionId || seconds < 1) return;
      pendingSeconds -= seconds;
      void request('/watch-events',{session_id:sessionId,event_id:crypto.randomUUID(),seconds},'POST',{},true).catch(()=>{});
    }
    function stopCounting(send=true) {
      account();
      active = false;
      if (send) flush();
    }
    function startCounting() {
      account();
      active = visible && !element.paused && !element.seeking && element.readyState >= 3;
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
      if (Date.now() < expiresAt - 30000) return;
      // Do not let HLS retry old cookies after a long pause.
      renewingForResume=true;
      stopCounting();
      element.pause();
      hls?.stopLoad();
      try {
        await grant();
        if (!stopped) {hls?.startLoad(); await element.play().catch(()=>{}); startCounting();}
      } catch(error) {if(!stopped)onerror((error as Error).message);}
      finally {renewingForResume=false;}
    }
    function onVisibility() {
      account();
      visible = !document.hidden;
      if (!visible) stopCounting();
      else if (element.readyState >= 3) startCounting();
    }
    const listeners: [string,EventListener][] = [
      ['play',()=>{void onPlay();}],
      ['playing',()=>{if(!renewingForResume)startCounting();}],
      ['pause',()=>stopCounting()],
      ['ended',()=>stopCounting()],
      ['waiting',()=>stopCounting()],
      ['stalled',()=>{if(element.readyState<3)stopCounting();}],
      ['seeking',()=>{active=false;lastClock=performance.now();lastMediaTime=element.currentTime;flush();}],
      ['seeked',()=>{if(element.readyState>=3)startCounting();}]
    ];
    for (const [event,listener] of listeners) element.addEventListener(event,listener);
    document.addEventListener('visibilitychange',onVisibility);
    window.addEventListener('pagehide',flush);

    // Analytics are best effort and never gate playback. Anonymous sessions are
    // intentionally unavailable; their 401 response does not interrupt viewing.
    void request('/watch-sessions',{video_id:videoId}).then(result=>{
      if (!stopped) {sessionId=result.id || result.session_id;pendingSeconds=0;lastClock=performance.now();lastMediaTime=element.currentTime;}
    }).catch(()=>{});
    void (async()=>{
      try {
        const result=await grant();
        if (stopped) return;
        if (element.canPlayType('application/vnd.apple.mpegurl') || !result.url.includes('.m3u8')) {
          element.src=result.url;
        } else {
          const Hls=(await import('hls.js')).default;
          if (stopped) return;
          if (!Hls.isSupported()) throw new Error('This browser does not support this video format.');
          hls=new Hls({xhrSetup(xhr){xhr.withCredentials=true;}});
          hls.on(Hls.Events.ERROR,(_,event)=>{
            if(event.fatal && !renewingForResume && !stopped)onerror('Video playback was interrupted. Please try again.');
          });
          hls.loadSource(result.url);
          hls.attachMedia(element);
        }
        await element.play().catch(()=>{});
      } catch(error) {if(!stopped)onerror((error as Error).message);}
    })();
    const timer=setInterval(()=>{
      account();
      const now=performance.now();
      if(now-lastHeartbeat>=15000){flush();lastHeartbeat=now;}
      if(active && Date.now()>=expiresAt-45000 && !grantRequest) {
        void grant().catch(error=>{if(!stopped)onerror((error as Error).message);});
      }
    },1000);
    return ()=>{
      stopCounting();
      stopped=true;
      clearInterval(timer);
      for(const [event,listener] of listeners)element.removeEventListener(event,listener);
      document.removeEventListener('visibilitychange',onVisibility);
      window.removeEventListener('pagehide',flush);
      hls?.destroy();
    };
  });
</script>
<video bind:this={element} controls playsinline crossorigin="use-credentials" onerror={()=>onerror('Video could not be played. Please try again.')}>{#if captionsUrl}<track kind="captions" src={captionsUrl} srclang="und" label="Captions" default/>{/if}</video>
