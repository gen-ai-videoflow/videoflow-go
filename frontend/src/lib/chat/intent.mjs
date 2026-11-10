const supported = new Set([
 'generate_script','generate_scenes','generate_shots','storyboard','render_scene',
 'render_shot','caption','retry_render','generate_assets','refine_scene','refine_shot',
 'delete_scene','style_ingest','plan_assets'
]);
/** Normalize both current replies and cards restored from older browser sessions. */
export function normalizeChatIntent(intent) {
 let action=intent.action;
 if(action==='create_script'||action==='add_scene')action='generate_script';
 return {...intent,action:supported.has(action)?action:'unknown'};
}
