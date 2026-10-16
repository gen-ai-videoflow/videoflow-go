import {beforeNavigate,goto} from '$app/navigation';
import {confirmAction} from './confirmation.svelte';
/** Register in component initialization; native unload warnings remain browser-controlled. */
export function guardUnsaved(dirty:()=>boolean,message:string){
 let allowed='';let pending=false;
 beforeNavigate(navigation=>{
  if(!dirty()||navigation.to?.url.href===allowed)return;
  navigation.cancel();
  if(navigation.willUnload||pending)return;
  const href=navigation.to?.url.href;if(!href)return;
  pending=true;
  void confirmAction(message).then(async accepted=>{if(accepted){allowed=href;try{await goto(href);}finally{allowed='';}}}).finally(()=>pending=false);
 });
}
