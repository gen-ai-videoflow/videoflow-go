/** Native WebAuthn wire conversion. Private keys never leave the authenticator. */
function decode(value:string):ArrayBuffer {
  const raw=atob(value.replace(/-/g,'+').replace(/_/g,'/'));
  return Uint8Array.from(raw,char=>char.charCodeAt(0)).buffer;
}
function encode(value:ArrayBuffer|null):string|null {
  if(value===null)return null;
  return btoa(String.fromCharCode(...new Uint8Array(value))).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
}
function supported(){
  if(!window.isSecureContext||!window.PublicKeyCredential||!navigator.credentials)throw new Error('Passkeys need a supported browser on a secure connection. Use your password to sign in instead.');
}
function wire(credential:PublicKeyCredential){
  const response=credential.response;
  const common={id:credential.id,rawId:encode(credential.rawId),type:credential.type,authenticatorAttachment:credential.authenticatorAttachment,clientExtensionResults:credential.getClientExtensionResults()};
  if(response instanceof AuthenticatorAttestationResponse)return {...common,response:{clientDataJSON:encode(response.clientDataJSON),attestationObject:encode(response.attestationObject),transports:response.getTransports?.()||[]}};
  const assertion=response as AuthenticatorAssertionResponse;
  return {...common,response:{clientDataJSON:encode(response.clientDataJSON),authenticatorData:encode(assertion.authenticatorData),signature:encode(assertion.signature),userHandle:encode(assertion.userHandle)}};
}
export async function createPasskey(value:any){
  supported();
  try {
    const publicKey={...value,challenge:decode(value.challenge),user:{...value.user,id:decode(value.user.id)},excludeCredentials:(value.excludeCredentials||[]).map((item:any)=>({...item,id:decode(item.id)}))};
    const credential=await navigator.credentials.create({publicKey}) as PublicKeyCredential|null;
    if(!credential)throw new Error('No passkey was created. You can try again.');
    return wire(credential);
  }catch(error){throw passkeyError(error);}
}
export async function usePasskey(value:any){
  supported();
  try {
    const publicKey={...value,challenge:decode(value.challenge),allowCredentials:(value.allowCredentials||[]).map((item:any)=>({...item,id:decode(item.id)}))};
    const credential=await navigator.credentials.get({publicKey}) as PublicKeyCredential|null;
    if(!credential)throw new Error('No passkey was selected. You can try again.');
    return wire(credential);
  }catch(error){throw passkeyError(error);}
}
function passkeyError(error:unknown):Error {
  if(error instanceof DOMException){
    if(error.name==='NotAllowedError'||error.name==='AbortError')return new Error('Passkey sign-in was cancelled or timed out. Try again, or use your password.');
    if(error.name==='InvalidStateError')return new Error('This authenticator already has a passkey for this account. Try another authenticator or sign in with your existing passkey.');
    if(error.name==='SecurityError')return new Error('This address cannot use the configured passkeys. Open VideoFlow at its approved secure address.');
  }
  return error instanceof Error?error:new Error('The passkey could not be used. Try again in a supported browser.');
}
