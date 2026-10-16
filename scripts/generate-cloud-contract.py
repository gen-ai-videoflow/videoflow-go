#!/usr/bin/env python3
"""Emit the versioned cloud API contract and shared TypeScript wire types."""
import json
from pathlib import Path
root=Path(__file__).resolve().parents[1]
schemas={
 'PromptTemplate':{'id':'string','revision':'integer','category':'string','title_en':'string','title_zh':'string','body_en':'string','body_zh':'string','defaults':'object','updated_at':'string'},
 'MediaAsset':{'asset_id':'string','job_id':'string','origin':'string','status':'string','filename':'string','content_type':'string','bytes':'integer','width':'integer','height':'integer','duration_ms':'integer','measured':'boolean','has_audio':'boolean','error':'string','poster_available':'boolean','captions_available':'boolean','job_status':'string'},
 'AccountSession':{'id':'string','created_at':'string','last_seen_at':'string','expires_at':'string','current':'boolean'},
 'PaymentInvoice':{'id':'string','purpose':'string','status':'string','address':'string','network':'string','token':'string','token_address':'string','amount':'string','amount_net':'string','fee_amount':'string','price_cents':'integer','pack_id':'string','credits':'integer','expires_at':'string','created_at':'string'},
 'User':{'id':'string','email':'string','name':'string','workspace_id':'string','channel_id':'string','handle':'string','verified':'boolean'},
 'Project':{'id':'string','workspace_id':'string','name':'string','description':'string','created_at':'string'},
 'Quote':{'id':'string','credits':'integer','expires_at':'string'},
 'Job':{'can_cancel':'boolean','project_id':'string','kind':'string','id':'string','quote_id':'string','status':'string','credits':'integer','error':'string','created_at':'string','updated_at':'string'},
 'CreditEntry':{'id':'string','amount':'integer','kind':'string','description':'string','created_at':'string'},
 'Video':{'id':'string','channel_id':'string','job_id':'string','title':'string','description':'string','visibility':'string','audience':'string','status':'string','views':'integer','watch_seconds':'integer','created_at':'string','handle':'string','channel_name':'string','thumbnail_url':'string','source_asset_id':'string','format':'string','origin':'string','ai_generated':'boolean','width':'integer','height':'integer','duration_ms':'integer'},
 'Error':{'code':'string','message':'string','request_id':'string'},
 'StudioEntity':{'id':'string','project_id':'string','scene_id':'string','kind':'string','title':'string','content':'string','description':'string','prompt':'string','object_key':'string','duration':'number','position':'integer','metadata':'object','created_at':'string','updated_at':'string'},
}
objects={k:{'type':'object','properties':{n:{'type':t}for n,t in fields.items()}}for k,fields in schemas.items()}
def obj(fields,required=()):return {'type':'object','additionalProperties':False,'properties':fields,'required':list(required)}
S={'type':'string'};I={'type':'integer'};B={'type':'boolean'}
objects['PipelineScene']=obj({'title':S,'prompt':S,'duration':I,'status':S},('title','prompt','duration','status'))
objects['PipelineProgress']=obj({'stage':S,'status':S,'script':S,'scenes':{'type':'array','items':{'$ref':'#/components/schemas/PipelineScene'}}},('stage','status','script','scenes'))
objects['CanvasGraph']=obj({'nodes':{'type':'array','items':obj({'id':S,'type':S,'label':S,'data':{'type':'object'}},('id','type','label'))},'edges':{'type':'array','items':obj({'source':S,'target':S,'label':S},('source','target','label'))}},('nodes','edges'))
objects['EditorDocument']=obj({'revision':I,'document':{'type':['object','null']},'updated_at':S},('revision','document'))
objects['PaymentInvoice']['required']=['id','purpose','status']
objects['BillingHistoryEntry']=obj({'id':S,'provider':{'type':'string','enum':['stripe','cryptorails']},'product':{'type':'string','enum':['credits','premium']},'amount_cents':I,'currency':S,'status':S,'created_at':S,'paid_at':{'type':['string','null']},'management_url':S,'receipt_available':B},('id','provider','product','amount_cents','currency','status','created_at','paid_at','management_url','receipt_available'))
objects['BillingHistoryPage']=obj({'items':{'type':'array','items':{'$ref':'#/components/schemas/BillingHistoryEntry'}},'next_cursor':S},('items','next_cursor'))
runtime_fields={'story_models':{'type':'array','maxItems':20,'items':S},'video_models':{'type':'array','maxItems':20,'items':S},**{name:I for name in ('credits_per_second','story_credits','assembly_credits','provider_daily_budget_micros','provider_max_cost_micros','story_max_cost_micros','premium_price_cents','max_active_jobs')}}
objects['RuntimePolicy']=obj(runtime_fields,tuple(runtime_fields))
objects['RuntimePolicyRecord']=obj({'policy':{'$ref':'#/components/schemas/RuntimePolicy'},'revision':I,'updated_at':{'type':['string','null']},'source':S},('policy','revision','updated_at','source'))
objects['AuthProviders']=obj({'providers':{'type':'array','items':S},'linked':{'type':'array','items':S}},('providers','linked'))
objects['OAuthStart']=obj({'url':{'type':'string','format':'uri'}},('url',))
objects['StripeCheckout']=obj({'provider':{'type':'string','const':'stripe'},'url':{'type':'string','format':'uri'}},('provider','url'))
objects['CheckoutResult']={'oneOf':[{'$ref':'#/components/schemas/PaymentInvoice'},{'$ref':'#/components/schemas/StripeCheckout'}]}
objects['AITaskReadiness']=obj({'id':S,'provider':{'type':'string','const':'openrouter'},'adapter_supported':B,'configured':B,'availability':{'type':'string','enum':['unsupported','configuration_missing','quote_validation_required']},'selection':S,'missing':{'type':'array','items':S}},('id','provider','adapter_supported','configured','availability','selection','missing'))
objects['AIReadiness']=obj({'provider':{'type':'string','const':'openrouter'},'live_provider_verified':{'type':'boolean','const':False},'tasks':{'type':'array','items':{'$ref':'#/components/schemas/AITaskReadiness'}},'note':S},('provider','live_provider_verified','tasks','note'))
objects['AdminSetting']=obj({'key':S,'value':{'oneOf':[B,S]},'revision':I,'updated_at':S},('key','value','revision','updated_at'))
objects['ProfileImageUpload']=obj({'url':{'type':'string','format':'uri'}},('url',))

requests={
 'prompt_template':obj({'id':S,'revision':{'type':'integer','minimum':0},'category':{'type':'string','enum':['story','character','scene','shot','asset','video']},'title_en':S,'title_zh':S,'body_en':S,'body_zh':S,'defaults':{'type':'object','additionalProperties':S}},('revision','category')),
 'editor_document':obj({'revision':{'type':'integer','minimum':0},'document':{'type':'object'}},('revision','document')),
 'empty':obj({}),
 'payment_invoice':obj({'purpose':{'type':'string','enum':['credits','premium']},'pack_id':S},('purpose',)),
 'edge':obj({'source':S,'target':S,'label':{'type':'string','enum':['casts','uses','reference']}},('source','target','label')),
 'canvas':obj({'title':S,'name':S,'description':S,'summary':S,'prompt':S,'duration':{'type':'number'},'aspect_ratio':S,'camera':S,'movement':S,'shot_order':I,'scene_id':S,'object_key':S}),
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
 'checkout':obj({'pack_id':S,'provider':{'type':'string','enum':['stripe','cryptorails']}},('pack_id',)),
 'premium_checkout':obj({'provider':{'type':'string','enum':['stripe','cryptorails']}}),
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
 ('get','/projects/{project}/prompt-templates',None,'PromptTemplate[]',False),('put','/projects/{project}/prompt-templates','prompt_template','PromptTemplate',False),('delete','/projects/{project}/prompt-templates/{template}',None,None,False),
 ('get','/projects/{project}/editor',None,'EditorDocument',False),('put','/projects/{project}/editor','editor_document','EditorDocument',False),
 ('get','/payments/options',None,None,True),('post','/payments/invoices','payment_invoice','PaymentInvoice',False),('get','/payments/invoices',None,'PaymentInvoice[]',False),('get','/payments/invoices/{id}',None,'PaymentInvoice',False),('post','/payments/invoices/{id}/notify','empty','PaymentInvoice',False),
 ('get','/premium',None,None,True),('post','/premium/checkout','premium_checkout','CheckoutResult',False),('post','/premium/manage','empty',None,False),('get','/projects/{project}/graph',None,'CanvasGraph',False),('post','/projects/{project}/edges','edge',None,False),('delete','/projects/{project}/edges','edge',None,False),
 ('post','/auth/register','register','User',True),('post','/auth/login','login','User',True),('post','/auth/logout',None,None,False),
 ('post','/auth/forgot-password','email',None,True),('post','/auth/resend-verification','email',None,True),('post','/auth/verify-email','token',None,True),('post','/auth/reset-password','reset',None,True),
 ('get','/me',None,'User',False),('get','/projects',None,'Project[]',False),('post','/projects','project','Project',False),
 ('get','/billing/history',None,'BillingHistoryPage',False),('get','/credits',None,None,False),('get','/billing/packs',None,None,True),('post','/billing/checkout','checkout','CheckoutResult',False),
 ('get','/jobs',None,'Job[]',False),('get','/jobs/{id}',None,'Job',False),('get','/jobs/{id}/events',None,None,False),('get','/jobs/{id}/download',None,None,False),('post','/jobs/{id}/cancel',None,None,False),
 ('get','/projects/{project}/jobs',None,'Job[]',False),('get','/jobs/{id}/pipeline',None,'PipelineProgress',False),('post','/projects/{project}/pipelines/quotes','pipeline_quote','Quote',False),('post','/projects/{project}/assembly/quotes','assembly_quote','Quote',False),
 ('post','/generation/quotes','quote','Quote',False),('post','/generation/jobs','job','Job',False),
 ('get','/feed',None,'Video[]',True),('get','/search',None,'Video[]',True),('get','/videos',None,'Video[]',True),('get','/videos/{id}',None,'Video',True),('post','/videos','video','Video',False),('patch','/videos/{id}',None,None,False),('post','/videos/{id}/publish',None,None,False),('post','/videos/{id}/playback-grant',None,None,True),('post','/videos/{id}/report','report',None,False),
 ('get','/channel/videos',None,None,False),('get','/channel',None,None,False),('patch','/channel','channel',None,False),('get','/channels/{handle}',None,None,True),('post','/channels/{id}/follow',None,None,False),
 ('get','/memberships',None,None,False),('post','/memberships/checkout','membership',None,False),('get','/creator/membership-plan',None,None,False),('put','/creator/membership-plan','plan',None,False),('get','/creator/payout-account',None,None,False),('post','/creator/payout-account','payout',None,False),('get','/creator/analytics',None,None,False),('get','/creator/analytics/detail',None,None,False),('get','/creator/earnings',None,None,False),
 ('post','/watch-sessions','watch',None,False),('post','/watch-events','event',None,False),('post','/viewer/preferences','preference',None,False),('post','/assets/upload-grant','upload',None,False),
]
requests['reauth']=obj({'password':S,'code':S},('password',))
requests['mfa']=obj({'challenge':S,'code':S},('challenge','code'))
requests['totp_confirm']=obj({'code':S},('code',))
requests['passkey_register']=obj({'challenge':S,'credential':{'type':'object'},'name':S},('challenge','credential'))
requests['passkey_login']=obj({'challenge':S,'credential':{'type':'object'}},('challenge','credential'))
requests['profile']=obj({'name':S,'bio':S,'description':S,'location':S,'website':S,'avatar_url':S,'banner_url':S,'featured_video_id':S,'social_links':{'type':'array','items':obj({'label':S,'url':S},('label','url'))}})
routes.extend([
 ('get','/security',None,None,False),('get','/security/sessions',None,'AccountSession[]',False),('delete','/security/sessions/{id}','empty',None,False),('post','/security/sessions/revoke-others','empty',None,False),('post','/auth/mfa','mfa','User',True),
 ('post','/security/totp/begin','reauth',None,False),('post','/security/totp/confirm','totp_confirm',None,False),('post','/security/totp/disable','reauth',None,False),
 ('post','/security/passkeys/register/options','reauth',None,False),('post','/security/passkeys/register/verify','passkey_register',None,False),('delete','/security/passkeys/{id}','reauth',None,False),
 ('post','/auth/passkeys/options','empty',None,True),('post','/auth/passkeys/verify','passkey_login','User',True),
 ('get','/profile',None,None,False),('patch','/profile','profile',None,False),
])
requests['preferences']=obj({'captions_enabled':B,'caption_language':S,'autoplay_next':B,'shorts_autoplay':B,'reduce_motion':{'type':'string','enum':['system','reduce','no-preference']},'data_saving':B,'history_enabled':B})
requests['publication']=obj({'source_asset_id':S,'job_id':S,'title':S,'description':S,'visibility':{'type':'string','enum':['public','unlisted','private']},'audience':{'type':'string','enum':['free','members']},'format':{'type':'string','enum':['standard','short']},'ai_generated':B,'rights_confirmed':B},('title','visibility','audience'))
requests['progress']=obj({'position_ms':I},('position_ms',))
requests['video_upload']=obj({'filename':S,'content_type':{'type':'string','enum':['video/mp4','video/webm']},'bytes':{'type':'integer','minimum':1,'maximum':104857600},'checksum':{'type':'string','pattern':'^[a-f0-9]{64}$'},'rights_confirmed':B,'idempotency_key':S},('filename','content_type','bytes','checksum','rights_confirmed','idempotency_key'))
requests['upload_part']=obj({'part_number':{'type':'integer','minimum':1,'maximum':20}},('part_number',))
routes.extend([
 ('get','/settings/preferences',None,None,False),('patch','/settings/preferences','preferences',None,False),
 ('get','/jobs/{id}/output',None,'MediaAsset',False),('get','/media-assets/{id}',None,'MediaAsset',False),('get','/media-assets/{id}/download',None,None,False),('get','/media-assets/{id}/preview',None,None,False),('post','/publications','publication','Video',False),
 ('get','/videos/{id}/actions',None,None,True),('get','/videos/{id}/poster',None,None,True),('get','/videos/{id}/related',None,'Video[]',True),('put','/videos/{id}/progress','progress',None,False),('get','/viewer/saved',None,None,False),('get','/viewer/history',None,None,False),('delete','/channels/{id}/follow','empty',None,False),
 ('get','/video-uploads/options',None,None,False),('post','/video-uploads','video_upload',None,False),('get','/video-uploads/{id}',None,None,False),('post','/video-uploads/{id}/parts','upload_part',None,False),('post','/video-uploads/{id}/complete','empty',None,False),('delete','/video-uploads/{id}','empty',None,False),
])
for action in ('save','like'):
 for method in ('put','delete'):routes.append((method,'/videos/{id}/'+action,'empty',None,False))
for kind in ('scripts','characters','scenes','assets'):
 base='/projects/{project}/'+kind
 routes.extend([('get',base,None,'StudioEntity[]',False),('post',base,'studio','StudioEntity',False),('patch',base+'/{entity}','studio_patch','StudioEntity',False),('delete',base+'/{entity}',None,None,False)])
base='/projects/{project}/scenes/{scene}/shots'
routes.extend([('get',base,None,'StudioEntity[]',False),('post',base,'studio','StudioEntity',False),('patch',base+'/{entity}','studio_patch','StudioEntity',False),('delete',base+'/{entity}',None,None,False),('get','/projects/{project}/export',None,None,False),('post','/projects/import','archive','Project',False),('get','/jobs/{id}/captions',None,None,False),('put','/jobs/{id}/captions','captions',None,False),('delete','/viewer/history',None,None,False),('post','/account/deletion','reauth',None,False)])
for kind in ('scenes','shots','characters','assets'):
 base='/projects/{project}/canvas/'+kind
 routes.extend([('get',base,None,None,False),('post',base,'canvas',None,False),('get',base+'/{entity}',None,None,False),('patch',base+'/{entity}','canvas',None,False),('delete',base+'/{entity}',None,None,False)])
for base in ('/projects/{project}/scenes/{scene}/cast/{character}','/projects/{project}/shots/{shot}/assets/{asset}'):
 routes.extend([('post',base,None,None,False),('delete',base,None,None,False)])
requests['runtime_policy']=obj({'policy':{'$ref':'#/components/schemas/RuntimePolicy'},'revision':{'type':'integer','minimum':0},'reason':{'type':'string','minLength':3,'maxLength':1000}},('policy','revision','reason'))
requests['oauth_start']=obj({'link':B,'password':S,'code':S})
requests['profile_image']=obj({'kind':{'type':'string','enum':['avatar','banner']},'data':{'type':'string','contentEncoding':'base64','maxLength':4194304}},('kind','data'))
requests['admin_setting']=obj({'value':{'oneOf':[B,S]},'revision':{'type':'integer','minimum':1},'reason':{'type':'string','minLength':1,'maxLength':1000}},('value','revision','reason'))
requests['admin_restriction']=obj({'suspended':B,'reason':{'type':'string','minLength':1,'maxLength':1000}},('suspended','reason'))
requests['admin_block']=obj({'reason':{'type':'string','maxLength':1000}})
routes.extend([
 ('get','/auth/providers',None,'AuthProviders',True),('post','/auth/oauth/{provider}/start','oauth_start','OAuthStart',True),('get','/auth/oauth/{provider}/callback',None,None,True),('post','/auth/oauth/{provider}/callback',None,None,True),
 ('post','/auth/magic-link','email',None,True),('post','/auth/magic-link/verify','token','User',True),
 ('post','/profile/images','profile_image','ProfileImageUpload',False),('get','/profile-images/{id}',None,None,True),
 ('get','/admin/runtime-policy/versions',None,None,False),('get','/admin/runtime-policy',None,'RuntimePolicyRecord',False),('put','/admin/runtime-policy','runtime_policy','RuntimePolicyRecord',False),('post','/admin/runtime-policy/refresh-catalogues','empty',None,False),('get','/admin/me',None,None,False),('get','/admin/settings',None,'AdminSetting[]',False),('put','/admin/settings/{key}','admin_setting','AdminSetting',False),('get','/admin/capacity',None,None,False),('get','/admin/users/{id}/records/{kind}',None,None,False),('post','/admin/reports/{id}/resolve','admin_block',None,False),('post','/admin/videos/{id}/restore','admin_block',None,False),('get','/admin/readiness',None,None,False),('get','/admin/ai-readiness',None,'AIReadiness',False),('get','/admin/records/{kind}',None,None,False),('post','/admin/users/{id}/restriction','admin_restriction',None,False),('post','/admin/videos/{id}/block','admin_block',None,False),
])
# Creator reward lifecycle contracts; operational evidence remains admin-only.
reward_money_fields=('lifetime_confirmed_cents','confirmed_unvested_cents','vested_cents','available_cents','legacy_pending_cents','legacy_unbacked_cents','payout_reserved_cents','paid_cents')
objects['RewardBalance']=obj({**{k:I for k in reward_money_fields},'currency':S,'held':B,'estimated_additional_cents':{'type':['integer','null']},'estimate_status':S,'calculated_at':S},(*reward_money_fields,'currency','held','estimated_additional_cents','estimate_status','calculated_at'))
objects['RewardProgram']=obj({'enabled':B,'enrolled':B,'policy_version':S,'terms_version':S,'earning_window_days':I,'vesting_days':I,'confirmation':S,'payout_cadence':S,'individual_spending_cap':B,'currency':S,'estimate_is_guaranteed':B},('enabled','enrolled','policy_version','terms_version','earning_window_days','vesting_days','confirmation','payout_cadence'))
objects['RewardVideo']=obj({'video_id':S,'title':S,'status':S,'reason_code':S,'first_published_at':{'type':['string','null']},'window_ends_at':{'type':['string','null']}},('video_id','title','status','reason_code','first_published_at','window_ends_at'))
requests['reward_enroll']=obj({'policy_version':S,'accepted':{'type':'boolean','const':True}},('policy_version','accepted'))
requests['reward_funding']=obj({'id':S,'reference':S,'kind':{'enum':['generation','advertising','support']},'cents':{'type':'integer','minimum':1},'evidence_reference':S},('id','reference','kind','cents','evidence_reference'))
requests['reward_batch']=obj({'id':S,'day':{'type':'string','format':'date'},'source_id':S,'policy_version':S,'budget_cents':{'type':'integer','minimum':1},'evidence_reference':S,'weights':{'type':'object','maxProperties':1000,'additionalProperties':{'type':'integer','minimum':0}}},('id','day','source_id','policy_version','budget_cents','evidence_reference','weights'))
requests['reward_hold']=obj({'held':B,'reason':S},('held','reason'))
routes.extend([
 ('get','/creator/rewards/program',None,'RewardProgram',False),('get','/creator/rewards/summary',None,'RewardBalance',False),('get','/creator/rewards/videos',None,None,False),('get','/creator/rewards/videos/{id}',None,None,False),('get','/creator/rewards/payouts',None,None,False),('post','/creator/rewards/enroll','reward_enroll',None,False),
 ('get','/admin/openapi.json',None,None,False),('get','/admin/rewards',None,None,False),('post','/admin/rewards/funding','reward_funding',None,False),('post','/admin/rewards/funding/{id}/reconcile','empty',None,False),('post','/admin/rewards/batches','reward_batch',None,False),('get','/admin/rewards/batches/{id}',None,None,False),('post','/admin/rewards/batches/{id}/confirm','empty',None,False),('post','/admin/rewards/users/{id}/hold','reward_hold',None,False),
])
# Discovery, measured audience and sponsored campaigns.
discovery_fields={'revision':I,'enabled':B,**{k:I for k in ('watch_cap','view_cap','view_weight','follow_weight','like_weight','history_weight','candidate_limit','title_weight','description_weight','page_limit','signal_limit','history_days','freshness_seconds','freshness_weight')},'synonyms':{'type':'object','maxProperties':100,'additionalProperties':{'type':'string','minLength':1,'maxLength':100}}}
objects['DiscoveryPolicy']=obj(discovery_fields,tuple(k for k in discovery_fields if k!='synonyms'))
objects['AdminObjectList']=obj({'items':{'type':'array','items':{'type':'object'}}},('items',))
objects['DiscoveryIndexHealth']=obj({'fts_index_ready':B,'estimated_video_rows':I,'index_version':I,'maintenance':S,'cjk_analysis':S,'cache':S},('fts_index_ready','estimated_video_rows','index_version','maintenance','cjk_analysis','cache'))
objects['DiscoveryAudience']=obj({'daily':{'type':'array','items':obj({'date':S,'measured_seconds':I,'accepted_seconds':I,'excluded_events':I,'unique_viewers':I,'organic_seconds':{'type':['integer','null']},'paid_seconds':{'type':['integer','null']}},('date','measured_seconds','accepted_seconds','excluded_events','unique_viewers','organic_seconds','paid_seconds'))},'privacy_minimum':I,'metric_version':I,'traffic_provenance':S},('daily','privacy_minimum','metric_version','traffic_provenance'))
objects['DiscoveryPreview']=obj({'items':{'type':'array','items':{'$ref':'#/components/schemas/Video'}},'next_cursor':S,'ranking':S,'history_used':B,'candidate_limit':I,'policy_revision':I},('items','next_cursor','ranking','history_used','candidate_limit','policy_revision'))
objects['AudienceCorrection']=obj({'video_id':S,'views':I,'watch_seconds':I,'metric_version':I,'rebuilt_at':S,'view_correction_status':S},('video_id','views','watch_seconds','metric_version','rebuilt_at','view_correction_status'))
objects['ResourceID']=obj({'id':S},('id',))
campaign_fields={'id':S,'title':{'type':'string','minLength':1,'maxLength':200},'brief':{'type':'string','minLength':3,'maxLength':10000},'terms_version':S,'video_id':S,'source_id':S,'budget_cents':{'type':'integer','minimum':1,'maximum':100000000},'starts_at':{'type':'string','format':'date-time'},'ends_at':{'type':'string','format':'date-time'}}
objects['SponsorCampaign']=obj(campaign_fields,tuple(campaign_fields))
objects['SponsorCampaignList']=obj({'items':{'type':'array','items':obj({'campaign':{'$ref':'#/components/schemas/SponsorCampaign'},'accepted':B},('campaign','accepted'))}},('items',))
objects['SponsorReport']=obj({'measured_paid_seconds':I,'accepted_paid_seconds':I,'sessions':I,'approved_obligations_cents':I,'settlement':S},('measured_paid_seconds','accepted_paid_seconds','sessions','approved_obligations_cents','settlement'))
objects['SponsoredWatch']=obj({'id':S,'sponsored':{'type':'boolean','const':True},'disclosure':{'type':'string','const':'Sponsored'},'campaign_id':S},('id','sponsored','disclosure','campaign_id'))
objects['SponsorAccepted']=obj({'accepted':B},('accepted',));objects['SponsorApproved']=obj({'approved':B},('approved',))
requests['discovery_policy']=obj({'policy':{'$ref':'#/components/schemas/DiscoveryPolicy'},'reason':{'type':'string','minLength':3,'maxLength':1000}},('policy','reason'))
requests['discovery_override']=obj({'video_id':S,'channel_id':S,'search_query':{'type':'string','maxLength':200},'weight':{'type':'integer','minimum':-10000000,'maximum':10000000},'expires_at':{'type':'string','format':'date-time'},'reason':{'type':'string','minLength':3,'maxLength':1000}},('weight','expires_at','reason'))
requests['audience_exclusion']=obj({'event_id':S,'reason':{'type':'string','minLength':3,'maxLength':1000}},('event_id','reason'))
requests['sponsor_campaign']=obj({'campaign':obj({k:v for k,v in campaign_fields.items() if k!='id'},tuple(k for k in campaign_fields if k!='id')),'reason':{'type':'string','minLength':3,'maxLength':1000}},('campaign','reason'))
requests['sponsor_award']=obj({'cents':{'type':'integer','minimum':1},'evidence_reference':{'type':'string','minLength':3,'maxLength':1000}},('cents','evidence_reference'))
requests['sponsor_accept']=obj({'terms_version':S},('terms_version',))
requests['reward_batch']['properties'].update({'campaign_id':S,'campaign_award_id':S})
routes.extend([
 ('get','/admin/discovery/policy',None,'DiscoveryPolicy',False),('put','/admin/discovery/policy','discovery_policy','DiscoveryPolicy',False),
 ('get','/admin/discovery/history',None,'AdminObjectList',False),('get','/admin/discovery/overrides',None,'AdminObjectList',False),('post','/admin/discovery/overrides','discovery_override','ResourceID',False),
 ('get','/admin/discovery/preview',None,'DiscoveryPreview',False),('get','/admin/discovery/index',None,'DiscoveryIndexHealth',False),('get','/admin/discovery/audience',None,'DiscoveryAudience',False),('post','/admin/discovery/exclusions','audience_exclusion','AudienceCorrection',False),
 ('get','/admin/campaigns',None,'SponsorCampaignList',False),('post','/admin/campaigns','sponsor_campaign','SponsorCampaign',False),('get','/admin/campaigns/{id}/report',None,'SponsorReport',False),('get','/admin/campaigns/{id}/awards',None,'AdminObjectList',False),('post','/admin/campaigns/{id}/awards','sponsor_award','ResourceID',False),('post','/admin/campaign-awards/{id}/approve','empty','SponsorApproved',False),
 ('get','/creator/campaigns',None,'SponsorCampaignList',False),('post','/creator/campaigns/{id}/accept','sponsor_accept','SponsorAccepted',False),('post','/campaigns/{id}/watch','empty','SponsoredWatch',False),
])

exec((root/'scripts/cloud-reward-engine-contract.py').read_text(),globals())
routes.extend([('post','/webhooks/stripe',None,None,True),('post','/webhooks/cryptorails',None,None,True),('post','/admin/export/{kind}','admin_block',None,False)])
# Support, appeals, notifications and immutable late-evidence corrections.
objects.setdefault('AdminObjectList',obj({'items':{'type':'array','items':{'type':'object'}},'limit':I},('items','limit')))
objects.setdefault('ResourceID',obj({'id':S},('id',)))
objects['RewardAction']=obj({k:B for k in ('created','approved','reviewed','resolved','proposed','read')})
objects['RewardBalance']['properties']['reward_adjustments_cents']=I
support_fields={'id':{'type':'string','minLength':1,'maxLength':128},'title':{'type':'string','minLength':1,'maxLength':120},'criteria':{'type':'string','minLength':1,'maxLength':2000},'source_id':S,'budget_cents':{'type':'integer','minimum':1,'maximum':100000000},'grant_credits':{'type':'integer','minimum':1,'maximum':1000000},'service_cost_cents':{'type':'integer','minimum':1,'maximum':1000000},'creator_cap':{'type':'integer','minimum':1,'maximum':10},'grant_cap':{'type':'integer','minimum':1,'maximum':10000},'expiry_days':{'type':'integer','minimum':1,'maximum':365},'minimum_publications':{'type':'integer','minimum':1,'maximum':100},'reason':{'type':'string','maxLength':2000}}
requests['reward_support_program']=obj(support_fields,tuple(k for k in support_fields if k!='reason'))
requests['reward_support_claim']=obj({'evidence':{'type':'string','minLength':1,'maxLength':2000}},('evidence',))
requests['reward_support_reason']=obj({'reason':{'type':'string','minLength':1,'maxLength':2000}},('reason',))
requests['reward_support_review']=obj({'reason':{'type':'string','minLength':1,'maxLength':2000},'approve':B},('reason','approve'))
requests['reward_appeal']=obj({'subject':{'type':'string','enum':['eligibility','hold','allocation','support','payout']},'allocation_id':S,'evidence':{'type':'string','minLength':1,'maxLength':4000}},('subject','evidence'))
requests['reward_appeal_resolution']=obj({'resolution':{'type':'string','minLength':1,'maxLength':2000},'upheld':B},('resolution','upheld'))
correction_fields={'id':{'type':'string','minLength':1,'maxLength':128},'original_batch_id':S,'user_id':S,'cents':{'type':'integer','minimum':-100000000,'maximum':100000000,'not':{'const':0}},'source_id':S,'reason':{'type':'string','minLength':1,'maxLength':1000},'evidence_reference':{'type':'string','minLength':1,'maxLength':2000}}
requests['reward_correction']=obj(correction_fields,('id','original_batch_id','user_id','cents','reason','evidence_reference'))
requests['reward_correction_reason']=obj({'reason':{'type':'string','minLength':1,'maxLength':1000}},('reason',))
objects['RewardAppeal']=obj({'id':S,'subject':S,'allocation_id':{'type':['string','null']},'evidence':S,'status':{'type':'string','enum':['open','upheld','declined']},'resolution':S,'created_at':S,'resolved_at':{'type':['string','null']}},('id','subject','allocation_id','evidence','status','resolution','created_at','resolved_at'))
objects['RewardNotification']=obj({'id':S,'kind':{'type':'string','enum':['confirmation','hold','release','payout','appeal','support','correction']},'message':S,'read_at':{'type':['string','null']},'created_at':S},('id','kind','message','read_at','created_at'))
objects['CreatorRewardCorrection']=obj({'id':S,'original_batch_id':S,'cents':I,'reason':S,'approved_at':S,'vesting_seconds':I},('id','original_batch_id','cents','reason','approved_at','vesting_seconds'))
for name in ('RewardAppeal','RewardNotification','CreatorRewardCorrection'):
 objects[name+'List']=obj({'items':{'type':'array','items':{'$ref':'#/components/schemas/'+name}},'limit':I},('items','limit'))
routes.extend([
 ('get','/creator/rewards/support',None,'AdminObjectList',False),('post','/creator/rewards/support/{id}/claims','reward_support_claim','ResourceID',False),
 ('get','/creator/rewards/appeals',None,'RewardAppealList',False),('post','/creator/rewards/appeals','reward_appeal','ResourceID',False),
 ('get','/creator/rewards/notifications',None,'RewardNotificationList',False),('post','/creator/rewards/notifications/{id}/read','empty','RewardAction',False),
 ('get','/creator/rewards/corrections',None,'CreatorRewardCorrectionList',False),
 ('get','/admin/rewards/support',None,'AdminObjectList',False),('post','/admin/rewards/support','reward_support_program','RewardAction',False),
 ('post','/admin/rewards/support/{id}/approve','reward_support_reason','RewardAction',False),('post','/admin/rewards/support/claims/{id}/review','reward_support_review','RewardAction',False),
 ('get','/admin/rewards/appeals',None,'AdminObjectList',False),('post','/admin/rewards/appeals/{id}/resolve','reward_appeal_resolution','RewardAction',False),
 ('get','/admin/rewards/corrections',None,'AdminObjectList',False),('post','/admin/rewards/corrections','reward_correction','RewardAction',False),('post','/admin/rewards/corrections/{id}/approve','reward_correction_reason','RewardAction',False),
])

# Independently approved commercial prices for prospective purchases.
commercial_pack={'id':{'type':'string','pattern':'^[a-z0-9][a-z0-9_-]{0,63}$'},'name':{'type':'string','minLength':1,'maxLength':100},'credits':{'type':'integer','minimum':1,'maximum':100000000},'price_cents':{'type':'integer','minimum':100,'maximum':10000000},'currency':{'type':'string','const':'usd'}}
objects['CommercialPack']=obj(commercial_pack,tuple(commercial_pack))
commercial_fields={'packs':{'type':'array','minItems':1,'maxItems':20,'items':{'$ref':'#/components/schemas/CommercialPack'}},'target_gross_margin_bps':{'type':'integer','minimum':1000,'maximum':9000},'fee_reserve_bps':{'type':'integer','minimum':0,'maximum':5000},'media_reserve_bps':{'type':'integer','minimum':0,'maximum':5000}}
objects['CommercialPolicy']=obj(commercial_fields,tuple(commercial_fields))
objects['CommercialPreview']=obj({'minimum_unit_revenue_micros':{'type':'number'},'worst_pack_gross_margin_bps':I,'required_gross_margin_bps':I,'margin_ok':B,'basis':S},('minimum_unit_revenue_micros','worst_pack_gross_margin_bps','required_gross_margin_bps','margin_ok','basis'))
objects['CommercialPolicyState']=obj({'policy':{'$ref':'#/components/schemas/CommercialPolicy'},'revision':I,'updated_at':{'type':['string','null']},'source':S,'preview':{'anyOf':[{'$ref':'#/components/schemas/CommercialPreview'},{'type':'null'}]},'preview_status':{'type':'string','enum':['ready','generation_policy_required']}},('policy','revision','updated_at','source','preview','preview_status'))
requests['commercial_policy']=obj({'policy':{'$ref':'#/components/schemas/CommercialPolicy'},'revision':{'type':'integer','minimum':0},'reason':{'type':'string','minLength':3,'maxLength':1000}},('policy','revision','reason'))
requests['commercial_preview']={'$ref':'#/components/schemas/CommercialPolicy'}
requests['commercial_approve']=obj({'reason':{'type':'string','minLength':3,'maxLength':1000}},('reason',))
routes.extend([
 ('get','/admin/commercial-policy',None,'CommercialPolicyState',False),
 ('put','/admin/commercial-policy','commercial_policy','ResourceID',False),
 ('get','/admin/commercial-policy/history',None,'AdminObjectList',False),
 ('get','/admin/commercial-policy/proposals',None,'AdminObjectList',False),
 ('post','/admin/commercial-policy/preview','commercial_preview','CommercialPreview',False),
 ('post','/admin/commercial-policy/proposals/{id}/approve','commercial_approve','RewardAction',False),
])


requests['legacy_backing_propose']=obj({'earning_id':S,'source_id':S,'reason':{'type':'string','minLength':3,'maxLength':1000},'evidence_reference':S},('earning_id','source_id','reason','evidence_reference'))
requests['legacy_backing_approve']=obj({'reason':{'type':'string','minLength':3,'maxLength':1000}},('reason',))
objects['LegacyBackingApproved']=obj({'approved':B},('approved',))
routes.extend([('get','/admin/rewards/legacy-backing',None,'AdminObjectList',False),('post','/admin/rewards/legacy-backing','legacy_backing_propose','RewardProposalResult',False),('post','/admin/rewards/legacy-backing/{id}/approve','legacy_backing_approve','LegacyBackingApproved',False)])

paths={}
for method,path,body,response,anonymous in routes:
 params=[{'name':part[1:-1],'in':'path','required':True,'schema':S}for part in path.split('/')if part.startswith('{')]
 if path in ('/generation/jobs','/memberships/checkout','/billing/checkout','/premium/checkout','/payments/invoices'):
  params.append({'name':'Idempotency-Key','in':'header','required':True,'schema':{'type':'string','minLength':8,'maxLength':200 if path in ('/payments/invoices','/premium/checkout','/billing/checkout') else 128}})
 op={'operationId':method+'_'+path.strip('/').replace('/','_').replace('{','').replace('}','').replace('-','_'),'security':[] if anonymous else [{'session':[]}],'parameters':params,'responses':{'200':{'description':'Success'},'400':{'description':'Invalid input'},'401':{'description':'Authentication or authorization required'},'409':{'description':'State conflict'},'503':{'description':'Integration disabled or unavailable'}}}
 op['tags']=['Administration' if path.startswith('/admin/') else 'Creator rewards' if path.startswith('/creator/rewards/') else 'Authentication' if path.startswith('/auth/') or path.startswith('/security') else 'Payments' if path.startswith(('/billing','/payments','/premium','/memberships')) else 'Creator' if path.startswith(('/creator','/channel')) else 'Studio' if path.startswith(('/projects','/generation','/jobs','/studio')) else 'Watching and publishing']
 if path in ('/auth/login','/auth/magic-link/verify'):op['responses']['202']={'description':'Second factor required; challenge is not an authenticated session','content':{'application/json':{'schema':obj({'mfa_required':B,'challenge':S,'methods':{'type':'array','items':S}},('mfa_required','challenge','methods'))}}}

 if path=='/projects/{project}/prompt-templates/{template}':op['parameters'].append({'name':'revision','in':'query','required':True,'schema':{'type':'integer','minimum':1}})
 if path=='/billing/history':op['parameters'].extend([{'name':'cursor','in':'query','schema':S},{'name':'provider','in':'query','schema':{'type':'string','enum':['all','stripe','cryptorails']}},{'name':'product','in':'query','schema':{'type':'string','enum':['all','credits','premium']}},{'name':'limit','in':'query','schema':{'type':'integer','minimum':1,'maximum':50,'default':20}}])
 if path in ('/feed','/videos','/search','/admin/discovery/preview'):op['parameters'].extend([{'name':n,'in':'query','schema':S} for n in ('q','category','filter','format','cursor','rank')])
 if path in ('/feed','/videos','/search') and method=='get':op['responses']['200']={'description':'Legacy requests return an array (up to 60); requests with format, rank or cursor return a paginated object (20 per page). All use the same effective discovery policy and access filters.','content':{'application/json':{'schema':{'oneOf':[{'type':'array','items':{'$ref':'#/components/schemas/Video'}},{'$ref':'#/components/schemas/DiscoveryPreview'}]}}}}
 if path=='/admin/discovery/audience':op['parameters'].extend([{'name':k,'in':'query','schema':S,'description':'Supply exactly one video_id or channel_id; aggregate history covers 90 days.'} for k in ('video_id','channel_id')])
 if body:op['requestBody']={'required':True,'content':{'application/json':{'schema':requests[body]}}}
 if response and not (path in ('/feed','/videos','/search') and method=='get'):
  schema={'$ref':'#/components/schemas/'+response.removesuffix('[]')}
  if response.endswith('[]'):schema={'type':'array','items':schema}
  op['responses']['201' if path in ('/auth/register','/profile/images') else '200']={'description':'Success','content':{'application/json':{'schema':schema}}}
 if path in ('/feed','/videos','/search','/admin/discovery/preview'):op['responses']['200']={'description':'Legacy array without format/cursor; format-filtered requests return items and next_cursor','content':{'application/json':{'schema':{'oneOf':[{'type':'array','items':{'$ref':'#/components/schemas/Video'}},obj({'items':{'type':'array','items':{'$ref':'#/components/schemas/Video'}},'next_cursor':S},('items','next_cursor'))]}}}}
 if path.startswith('/auth/oauth/'):
  for param in op['parameters']:
   if param['name']=='provider':param['schema']={'type':'string','enum':['google','apple']}
 if path=='/auth/oauth/{provider}/start':op['description']='Starts configured OIDC with browser-bound state and nonce; Google uses S256 PKCE and Apple uses its confidential-client form-post flow. link=true requires the current authenticated account and password/second-factor reauthentication. Never links by email alone.'
 if path=='/auth/oauth/{provider}/callback':
  op['responses'].pop('200',None)
  op['responses']['303']={'description':'Redirect to application after state/nonce/issuer verification and optional second-factor challenge','headers':{'Location':{'schema':S}}}
  callback=obj({'state':S,'code':S,'error':S})
  if method=='post':op['requestBody']={'required':True,'content':{'application/x-www-form-urlencoded':{'schema':callback}}}
  else:op['parameters'].extend([{'name':name,'in':'query','schema':S} for name in ('state','code','error')])
 if path.startswith('/admin/'):
  op['description']='Requires an assigned administrator role, enrolled MFA/passkey and a verified session. Mutations require recent second-factor verification; role permissions are enforced server side.'
  op['responses']['403']={'description':'Administrator role, second factor or recent verification required'}
 if path=='/admin/records/{kind}':
  op['parameters'][0]['schema']={'type':'string','enum':['premium', 'crypto', 'policies', 'usage', 'budgets', 'revenue', 'audience', 'users', 'content', 'jobs', 'billing', 'reports', 'audit']}
 if path=='/profile-images/{id}':op['responses']['200']={'description':'Normalized public profile JPEG','content':{'image/jpeg':{'schema':{'type':'string','format':'binary'}}}}
 if path.endswith('/events'):
  op['responses']['200']={'description':'Authorized job snapshots as resumable progress hints','content':{'text/event-stream':{'schema':S}}}
 if path in ('/memberships/checkout','/creator/membership-plan'):
  op['deprecated']=True
  op['description']='Channel membership pricing is retired; use platform-wide Premium.'
  op['responses']['410']={'description':'Per-channel subscriptions are retired'}
 paths.setdefault(path,{})[method]=op
contract={'openapi':'3.1.0','info':{'title':'6eek Cloud API','version':'1.0.0'},'servers':[{'url':'https://api.6eek.com/v1'},{'url':'/api/v1'}],'paths':paths,'components':{'securitySchemes':{'session':{'type':'apiKey','in':'cookie','name':'6eek_session'}},'schemas':objects}}
(root/'api/openapi.json').write_text(json.dumps(contract,indent=2)+'\n')
ts=['// Generated by scripts/generate-cloud-contract.py. Do not edit by hand.']
for name,fields in schemas.items():
 ts.append('export interface '+name+' {\n'+'\n'.join('  '+k+('?' if k in ('error','thumbnail_url','request_id','scene_id','object_key','defaults')else '')+': '+{'integer':'number','number':'number','object':'Record<string, unknown>','string':'string','boolean':'boolean'}[v]+';'for k,v in fields.items())+'\n}')
ts.append("export interface RuntimePolicy { story_models: string[]; video_models: string[]; credits_per_second: number; story_credits: number; assembly_credits: number; provider_daily_budget_micros: number; provider_max_cost_micros: number; story_max_cost_micros: number; premium_price_cents: number; max_active_jobs: number; }\nexport interface RuntimePolicyRecord { policy: RuntimePolicy; revision: number; updated_at: string | null; source: string; }")
ts.append("export interface AuthProviders { providers: string[]; linked: string[]; }\nexport interface OAuthStart { url: string; }")
ts.append("export interface BillingHistoryEntry { id: string; provider: 'stripe' | 'cryptorails'; product: 'credits' | 'premium'; amount_cents: number; currency: string; status: string; created_at: string; paid_at: string | null; management_url: string; receipt_available: boolean; }\nexport interface BillingHistoryPage { items: BillingHistoryEntry[]; next_cursor: string; }")
ts.append("export interface StripeCheckout { provider: 'stripe'; url: string; }\nexport type CheckoutResult = PaymentInvoice | StripeCheckout;\nexport interface AITaskReadiness { id: string; provider: 'openrouter'; adapter_supported: boolean; configured: boolean; availability: 'unsupported' | 'configuration_missing' | 'quote_validation_required'; selection: string; missing: string[]; }\nexport interface AIReadiness { provider: 'openrouter'; live_provider_verified: false; tasks: AITaskReadiness[]; note: string; }\nexport interface AdminSetting { key: string; value: boolean | string; revision: number; updated_at: string; }\nexport interface ProfileImageUpload { url: string; }")
ts.append("export interface EditorDocument { revision: number; document: Record<string, unknown> | null; updated_at?: string; }")
ts.append("export interface PipelineScene { title: string; prompt: string; duration: number; status: string; }\nexport interface PipelineProgress { stage: string; status: string; script: string; scenes: PipelineScene[]; }")
ts.append("export type VideoVisibility = 'public' | 'unlisted' | 'private';\nexport type VideoAudience = 'free' | 'members';")
ts.append("export interface RewardBalance { currency: string; lifetime_confirmed_cents: number; confirmed_unvested_cents: number; vested_cents: number; available_cents: number; legacy_pending_cents: number; legacy_unbacked_cents: number; payout_reserved_cents: number; paid_cents: number; reward_adjustments_cents: number; held: boolean; estimated_additional_cents: number | null; estimate_status: string; calculated_at: string; }\nexport interface RewardProgram { enabled: boolean; enrolled: boolean; policy_version: string; terms_version: string; earning_window_days: number; vesting_days: number; confirmation: string; payout_cadence: string; }\nexport interface RewardVideo { video_id: string; title: string; status: string; reason_code: string; first_published_at: string | null; window_ends_at: string | null; }")
(root/('frontend/src/lib/cloud/generated.ts' if (root/'frontend').exists() else 'api/generated.ts')).write_text('\n\n'.join(ts)+'\n')
print(f'Generated {len(routes)} operations and {len(objects)} shared types.')
