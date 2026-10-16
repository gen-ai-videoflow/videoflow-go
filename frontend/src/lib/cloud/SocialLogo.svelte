<script lang="ts">let {name,size=16}:{name:string;size?:number}=$props();</script>
<svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
{#if name==='YouTube'}<path d="M23 7s-.2-1.6-.9-2.3c-.9-.9-1.9-.9-2.3-1C16.6 3.5 12 3.5 12 3.5s-4.6 0-7.8.2c-.4.1-1.4.1-2.3 1C1.2 5.4 1 7 1 7S.8 8.9.8 10.8v2.4c0 1.9.2 3.8.2 3.8s.2 1.6.9 2.3c.9.9 2.1.9 2.7 1 2 .2 7.4.2 7.4.2s4.6 0 7.8-.2c.4-.1 1.4-.1 2.3-1 .7-.7.9-2.3.9-2.3s.2-1.9.2-3.8v-2.4C23.2 8.9 23 7 23 7ZM9.5 16V8l7 4-7 4Z"/>
{:else if name==='Instagram'}<rect x="3" y="3" width="18" height="18" rx="5" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="4" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="17.5" cy="6.5" r="1.2"/>
{:else if name==='TikTok'}<path d="M16.7 2h-3.2v13.4a3 3 0 1 1-2.6-3V9.1a6.3 6.3 0 1 0 5.8 6.3V8.6a8 8 0 0 0 5 1.7V7.1A5 5 0 0 1 16.7 2Z"/>
{:else if name==='LinkedIn'}<path d="M4 3a2 2 0 1 0 0 4 2 2 0 0 0 0-4ZM2.3 9h3.4v12H2.3V9ZM9 9h3.3v1.6A4.2 4.2 0 0 1 16 8.7c3.4 0 4.3 2.2 4.3 5.1V21h-3.4v-6.4c0-1.5 0-3.1-1.9-3.1s-2.6 1.5-2.6 3V21H9V9Z"/>
{:else if name==='GitHub'}<path d="M12 .8a11.4 11.4 0 0 0-3.6 22.2c.6.1.8-.2.8-.6v-2.1c-3.3.7-4-1.4-4-1.4-.5-1.3-1.3-1.6-1.3-1.6-1.1-.7.1-.7.1-.7 1.2.1 1.8 1.2 1.8 1.2 1 1.8 2.7 1.3 3.4 1 .1-.8.4-1.3.7-1.6-2.6-.3-5.3-1.3-5.3-5.7 0-1.3.4-2.3 1.2-3.1-.1-.3-.5-1.5.1-3.1 0 0 1-.3 3.2 1.2a11 11 0 0 1 5.8 0C17.2 5 18.2 5.3 18.2 5.3c.6 1.6.2 2.8.1 3.1.8.8 1.2 1.8 1.2 3.1 0 4.4-2.7 5.4-5.3 5.7.4.4.8 1.1.8 2.2v3c0 .4.2.7.8.6A11.4 11.4 0 0 0 12 .8Z"/>
{:else if name==='X'}<path d="m3 3 18 18M21 3 3 21" fill="none" stroke="currentColor" stroke-width="2.5"/>
{:else if name==='Facebook'}<path d="M14 22v-9h3l.5-4H14V7c0-1 .5-2 2-2h2V1h-3c-4 0-6 2-6 6v2H6v4h3v9z"/>
{:else if name==='Twitch'}<path d="M4 2h18v14l-6 6h-5l-3 2v-2H2V6z" fill="none" stroke="currentColor" stroke-width="2"/><path d="M10 6v7m6-7v7" stroke="currentColor" stroke-width="2"/>
{:else if name==='Discord'}<path d="M6 5c4-2 8-2 12 0 3 4 4 8 3 12l-5 2-1-2H9l-1 2-5-2C2 13 3 9 6 5Z" fill="none" stroke="currentColor" stroke-width="1.7"/><circle cx="8" cy="12" r="1.5"/><circle cx="16" cy="12" r="1.5"/>
{:else if name==='Telegram'}<path d="m2 11 20-8-4 18-6-5-3 3 .5-6L19 6 7 12z"/>
{:else}<circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" stroke-width="1.8"/><ellipse cx="12" cy="12" rx="4" ry="9" fill="none" stroke="currentColor" stroke-width="1.8"/><path d="M3 12h18" stroke="currentColor" stroke-width="1.8"/>{/if}
</svg>
