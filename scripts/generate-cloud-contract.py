#!/usr/bin/env python3
"""Emit the versioned cloud API contract and shared TypeScript wire types."""
import json
from pathlib import Path
root=Path(__file__).resolve().parents[1]
schemas={
 'User':{'id':'string','email':'string','name':'string','workspace_id':'string','channel_id':'string','handle':'string','verified':'boolean'},
 'Project':{'id':'string','workspace_id':'string','name':'string','description':'string','created_at':'string'},
 'Quote':{'id':'string','credits':'integer','expires_at':'string'},
 'Job':{'can_cancel':'boolean','project_id':'string','kind':'string','id':'string','quote_id':'string','status':'string','credits':'integer','error':'string','created_at':'string','updated_at':'string'},
 'CreditEntry':{'id':'string','amount':'integer','kind':'string','description':'string','created_at':'string'},
 'Video':{'id':'string','channel_id':'string','job_id':'string','title':'string','description':'string','visibility':'string','audience':'string','status':'string','views':'integer','watch_seconds':'integer','created_at':'string','handle':'string','channel_name':'string','thumbnail_url':'string'},
 'Error':{'code':'string','message':'string','request_id':'string'},
 'StudioEntity':{'id':'string','project_id':'string','scene_id':'string','kind':'string','title':'string','content':'string','description':'string','prompt':'string','object_key':'string','duration':'number','position':'integer','metadata':'object','created_at':'string','updated_at':'string'},
}
objects={k:{'type':'object','properties':{n:{'type':t}for n,t in fields.items()}}for k,fields in schemas.items()}
def obj(fields,required=()):return {'type':'object','additionalProperties':False,'properties':fields,'required':list(required)}
S={'type':'string'};I={'type':'integer'};B={'type':'boolean'}
objects['PipelineScene']=obj({'title':S,'prompt':S,'duration':I,'status':S},('title','prompt','duration','status'))
objects['PipelineProgress']=obj({'stage':S,'status':S,'script':S,'scenes':{'type':'array','items':{'$ref':'#/components/schemas/PipelineScene'}}},('stage','status','script','scenes'))
requests={
 'studio':obj({'title':{'type':'string','maxLength':180},'content':{'type':'string','maxLength':100000},'description':{'type':'string','maxLength':8000},'prompt':{'type':'string','maxLength':8000},'object_key':S,'duration':{'type':'number','minimum':0,'maximum':3600},'position':{'type':'integer','minimum':0,'maximum':10000},'metadata':{'type':'object'}},('title',)),
 'studio_patch':{'type':'object','additionalProperties':False,'properties':{x:{'type':t}for x,t in {'title':'string','content':'string','description':'string','prompt':'string','duration':'number','position':'integer','metadata':'object'}.items()}},
 'captions':obj({'segments':{'type':'array','items':obj({'start':{'type':'number'},'end':{'type':'number'},'text':S},('start','end','text'))}},('segments',)),
 'archive':obj({'version':{'type':'integer','const':1},'name':S,'project':obj({'name':S,'description':S},('name',)),'entities':{'type':'array','items':{'$ref':'#/components/schemas/StudioEntity'}}},('version','project','entities')),

 'register':obj({x:S for x in ('email','password','name','handle')},('email','password','name','handle')),
 'login':obj({'email':S,'password':S},('email','password')),
 'project':obj({'name':S,'description':S},('name',)),
 'quote':obj({'project_id':S,'prompt':S,'duration':{'type':'integer','minimum':5,'maximum':30},'aspect_ratio':{'type':'string','enum':['16:9','9:16','1:1']}},('project_id','prompt','duration','aspect_ratio')),
 'pipeline_quote':obj({'idea':{'type':'string','maxLength':8000},'clips':{'type':'integer','minimum':1,'maximum':4},'clip_duration':{'type':'integer','minimum':5,'maximum':10},'aspect_ratio':{'type':'string','enum':['16:9','9:16','1:1']}},('idea','clips','clip_duration','aspect_ratio')),
 'assembly_quote':obj({'job_ids':{'type':'array','minItems':1,'maxItems':8,'items':S}},('job_ids',)),
 'job':obj({'quote_id':S},('quote_id',)),
 'checkout':obj({'pack_id':S},('pack_id',)),
 'email':obj({'email':S},('email',)),
 'token':obj({'token':S},('token',)),
 'reset':obj({'token':S,'password':S},('token','password')),
 'channel':obj({'name':S,'description':S},('name',)),
 'video':obj({'job_id':S,'title':S,'description':S,'visibility':{'type':'string','enum':['public','unlisted','private']},'audience':{'type':'string','enum':['free','members']}},('job_id','title','visibility','audience')),
 'upload':obj({'filename':S,'content_type':S,'bytes':{'type':'integer','minimum':1,'maximum':104857600}},('filename','content_type','bytes')),
 'watch':obj({'video_id':S},('video_id',)),
 'event':obj({'session_id':S,'event_id':S,'seconds':{'type':'integer','minimum':1,'maximum':15}},('session_id','event_id','seconds')),
 'report':obj({'reason':{'type':'string','minLength':5,'maxLength':2000}},('reason',)),
 'membership':obj({'channel_id':S},('channel_id',)),
 'plan':obj({'price_cents':I,'currency':{'type':'string','enum':['usd']},'enabled':B},('price_cents','currency','enabled')),
 'payout':obj({'country':{'type':'string','minLength':2,'maxLength':2}},('country',)),
 'preference':obj({'channel_id':S,'hidden':B},('channel_id','hidden')),
}
# method, route, request schema, response schema, anonymous
routes=[
 ('post','/auth/register','register','User',True),('post','/auth/login','login','User',True),('post','/auth/logout',None,None,False),
 ('post','/auth/forgot-password','email',None,True),('post','/auth/resend-verification','email',None,True),('post','/auth/verify-email','token',None,True),('post','/auth/reset-password','reset',None,True),
 ('get','/me',None,'User',False),('get','/projects',None,'Project[]',False),('post','/projects','project','Project',False),
 ('get','/credits',None,None,False),('get','/billing/packs',None,None,True),('post','/billing/checkout','checkout',None,False),
 ('get','/jobs',None,'Job[]',False),('get','/jobs/{id}',None,'Job',False),('get','/jobs/{id}/events',None,None,False),('get','/jobs/{id}/download',None,None,False),('post','/jobs/{id}/cancel',None,None,False),
 ('get','/projects/{project}/jobs',None,'Job[]',False),('get','/jobs/{id}/pipeline',None,'PipelineProgress',False),('post','/projects/{project}/pipelines/quotes','pipeline_quote','Quote',False),('post','/projects/{project}/assembly/quotes','assembly_quote','Quote',False),
 ('post','/generation/quotes','quote','Quote',False),('post','/generation/jobs','job','Job',False),
 ('get','/feed',None,'Video[]',True),('get','/search',None,'Video[]',True),('get','/videos',None,'Video[]',True),('get','/videos/{id}',None,'Video',True),('post','/videos','video','Video',False),('patch','/videos/{id}',None,None,False),('post','/videos/{id}/publish',None,None,False),('post','/videos/{id}/playback-grant',None,None,True),('post','/videos/{id}/report','report',None,False),
 ('get','/channel',None,None,False),('patch','/channel','channel',None,False),('get','/channels/{handle}',None,None,True),('post','/channels/{id}/follow',None,None,False),
 ('get','/memberships',None,None,False),('post','/memberships/checkout','membership',None,False),('get','/creator/membership-plan',None,None,False),('put','/creator/membership-plan','plan',None,False),('get','/creator/payout-account',None,None,False),('post','/creator/payout-account','payout',None,False),('get','/creator/analytics',None,None,False),('get','/creator/earnings',None,None,False),
 ('post','/watch-sessions','watch',None,False),('post','/watch-events','event',None,False),('post','/viewer/preferences','preference',None,False),('post','/assets/upload-grant','upload',None,False),
]
for kind in ('scripts','characters','scenes','assets'):
 base='/projects/{project}/'+kind
 routes.extend([('get',base,None,'StudioEntity[]',False),('post',base,'studio','StudioEntity',False),('patch',base+'/{entity}','studio_patch','StudioEntity',False),('delete',base+'/{entity}',None,None,False)])
base='/projects/{project}/scenes/{scene}/shots'
routes.extend([('get',base,None,'StudioEntity[]',False),('post',base,'studio','StudioEntity',False),('patch',base+'/{entity}','studio_patch','StudioEntity',False),('delete',base+'/{entity}',None,None,False),('get','/projects/{project}/export',None,None,False),('post','/projects/import','archive','Project',False),('get','/jobs/{id}/captions',None,None,False),('put','/jobs/{id}/captions','captions',None,False),('delete','/viewer/history',None,None,False),('post','/account/deletion',None,None,False)])
paths={}
for method,path,body,response,anonymous in routes:
 params=[{'name':part[1:-1],'in':'path','required':True,'schema':S}for part in path.split('/')if part.startswith('{')]
 if path in ('/generation/jobs','/memberships/checkout','/billing/checkout'):
  params.append({'name':'Idempotency-Key','in':'header','required':True,'schema':{'type':'string','minLength':8,'maxLength':128}})
 op={'operationId':method+'_'+path.strip('/').replace('/','_').replace('{','').replace('}','').replace('-','_'),'security':[] if anonymous else [{'session':[]}],'parameters':params,'responses':{'200':{'description':'Success'},'400':{'description':'Invalid input'},'401':{'description':'Authentication or authorization required'},'409':{'description':'State conflict'},'503':{'description':'Integration disabled or unavailable'}}}
 if body:op['requestBody']={'required':True,'content':{'application/json':{'schema':requests[body]}}}
 if response:
  schema={'$ref':'#/components/schemas/'+response.removesuffix('[]')}
  if response.endswith('[]'):schema={'type':'array','items':schema}
  op['responses']['201' if path=='/auth/register' else '200']={'description':'Success','content':{'application/json':{'schema':schema}}}
 if path.endswith('/events'):
  op['responses']['200']={'description':'Authorized job snapshots as resumable progress hints','content':{'text/event-stream':{'schema':S}}}
 paths.setdefault(path,{})[method]=op
contract={'openapi':'3.1.0','info':{'title':'VideoFlow Cloud API','version':'1.0.0'},'servers':[{'url':'/api/v1'}],'paths':paths,'components':{'securitySchemes':{'session':{'type':'apiKey','in':'cookie','name':'videoflow_session'}},'schemas':objects}}
(root/'api/openapi.json').write_text(json.dumps(contract,indent=2)+'\n')
ts=['// Generated by scripts/generate-cloud-contract.py. Do not edit by hand.']
for name,fields in schemas.items():
 ts.append('export interface '+name+' {\n'+'\n'.join('  '+k+('?' if k in ('error','thumbnail_url','request_id','scene_id','object_key')else '')+': '+{'integer':'number','number':'number','object':'Record<string, unknown>','string':'string','boolean':'boolean'}[v]+';'for k,v in fields.items())+'\n}')
ts.append("export interface PipelineScene { title: string; prompt: string; duration: number; status: string; }\nexport interface PipelineProgress { stage: string; status: string; script: string; scenes: PipelineScene[]; }")
ts.append("export type VideoVisibility = 'public' | 'unlisted' | 'private';\nexport type VideoAudience = 'free' | 'members';")
(root/('frontend/src/lib/cloud/generated.ts' if (root/'frontend').exists() else 'api/generated.ts')).write_text('\n\n'.join(ts)+'\n')
print(f'Generated {len(routes)} operations and {len(objects)} shared types.')
