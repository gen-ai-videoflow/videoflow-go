export const confirmation = $state<{open:boolean;message:string;resolve?: (value:boolean)=>void}>({open:false,message:''});
export function confirmAction(message:string):Promise<boolean>{
  if(confirmation.open)return Promise.resolve(false);
  confirmation.message=message;confirmation.open=true;
  return new Promise(resolve=>confirmation.resolve=resolve);
}
export function finishConfirmation(value:boolean){const resolve=confirmation.resolve;confirmation.open=false;confirmation.resolve=undefined;resolve?.(value);}
