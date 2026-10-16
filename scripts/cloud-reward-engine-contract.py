"""Reward engine contract extension, evaluated by generate-cloud-contract.py."""
fields={'enabled':B,'earning_window_days':{'type':'integer','minimum':1,'maximum':90},'vesting_days':{'type':'integer','minimum':1,'maximum':365},'payout_threshold_cents':{'type':'integer','minimum':100,'maximum':10000000},'payout_interval_days':{'type':'integer','minimum':1,'maximum':90},'funding_bps':{'type':'integer','minimum':0,'maximum':10000},'reserve_bps':{'type':'integer','minimum':0,'maximum':10000},'daily_cap_cents':{'type':'integer','minimum':0,'maximum':100000000},'minimum_qualified_views':{'type':'integer','minimum':1,'maximum':1000000},'quality_weights':obj({k:{'type':'integer','minimum':0,'maximum':10000} for k in ('watch','retention','originality','trust')},('watch','retention','originality','trust')),'estimates_enabled':B,'confirmations_enabled':B,'payouts_enabled':B}
objects['RewardEnginePolicy']=obj(fields,tuple(fields))
objects['RewardEnginePolicyRecord']=obj({'policy':{'$ref':'#/components/schemas/RewardEnginePolicy'},'revision':I,'policy_version':S,'updated_at':{'type':['string','null']}},('policy','revision','policy_version','updated_at'))
objects['RewardCalculationResult']=obj({'run_id':S,'status':S,'videos':I,'estimated_cents':I,'draft_batches':I},('run_id','status','videos','estimated_cents','draft_batches'))
objects['RewardHealth']=obj({**{k:I for k in ('funded_cents','reserved_cents','available_cents','confirmed_cents','unvested_cents','held_accounts','failed_runs','legacy_pending_cents','recovery_reserved_cents')},'latest_run':{'type':['object','null']}},('funded_cents','reserved_cents','available_cents','confirmed_cents','unvested_cents','held_accounts','failed_runs','legacy_pending_cents','recovery_reserved_cents','latest_run'))
objects['RewardItems']=obj({'items':{'type':'array','maxItems':100,'items':{'type':'object'}}},('items',))
objects['RewardProposalResult']=obj({'id':S,'status':S},('id','status'))
objects['RewardPayoutRun']=obj({'id':S,'status':S,'batches':I,'failures':I},('id','status','batches','failures'))
objects['RewardCreatorDetail']=obj({'user_id':S,**{k:{'type':'array','maxItems':100,'items':{'type':'object'}} for k in ('allocations','earnings','payouts','holds')}},('user_id','allocations','earnings','payouts','holds'))
reason={'type':'string','minLength':1,'maxLength':1000}
requests['reward_engine_policy']=obj({'policy':{'$ref':'#/components/schemas/RewardEnginePolicy'},'revision':{'type':'integer','minimum':0},'reason':reason},('policy','revision','reason'))
requests['reward_calculate']=obj({'day':{'type':'string','format':'date'},'shadow':B,'reason':reason},('day','shadow','reason'))
requests['reward_reason']=obj({'reason':reason},('reason',))
requests['reward_quality']=obj({'originality_bps':{'type':'integer','minimum':0,'maximum':10000},'trust_bps':{'type':'integer','minimum':0,'maximum':10000},'eligible':B,'revision':{'type':'integer','minimum':0},'reason':reason},('originality_bps','trust_bps','eligible','revision','reason'))
requests['reward_transfer_reconcile']=obj({'transfer_id':{'type':'string','pattern':'^tr_','maxLength':200},'reason':reason},('transfer_id','reason'))
routes.extend([
 ('get','/admin/rewards/policy',None,'RewardEnginePolicyRecord',False),('put','/admin/rewards/policy','reward_engine_policy','RewardProposalResult',False),('get','/admin/rewards/policy/proposals',None,'RewardItems',False),('post','/admin/rewards/policy/proposals/{id}/approve','empty',None,False),
 ('get','/admin/rewards/health',None,'RewardHealth',False),('get','/admin/rewards/calculations',None,'RewardItems',False),('post','/admin/rewards/calculate','reward_calculate','RewardCalculationResult',False),('get','/admin/rewards/reviews',None,'RewardItems',False),('get','/admin/rewards/quality',None,'RewardItems',False),('get','/admin/rewards/quality/{id}',None,None,False),('put','/admin/rewards/quality/{id}','reward_quality',None,False),
 ('get','/admin/rewards/users/{id}',None,'RewardCreatorDetail',False),('get','/admin/rewards/payout-runs',None,'RewardItems',False),('post','/admin/rewards/payout-runs','reward_reason','RewardPayoutRun',False),('post','/admin/rewards/payouts/{id}/reconcile','reward_transfer_reconcile',None,False),
])
# Revenue evidence is a reviewed statement, never an assumed token/USD rate.
requests['reward_revenue_evidence']=obj({'id':S,'kind':{'enum':['receipt','cost','reversal','recovery']},'resource_id':S,'reason':reason,'provider':S,'replacement_source_id':S,**{k:{'type':['integer','null'],'minimum':0} for k in ('gross_cents','fee_cents','tax_cents','refund_cents','provider_cost_cents','media_cost_cents')},'settled_at':S,'evidence_reference':S},('kind','resource_id','reason','evidence_reference'))
requests['reward_revenue_day']=obj({'day':{'type':'string','format':'date'},'reason':reason},('day','reason'))
objects['RewardRevenueResult']=obj({'day':S,'status':S,'jobs':I,'pending':I,'net_cents':I,'funded_cents':I,'source_id':S,'message':S},('day','status','jobs','pending','net_cents','funded_cents'))
routes.extend([('get','/admin/rewards/revenue',None,'RewardItems',False),('get','/admin/rewards/revenue/stripe/{id}/statement',None,None,False),('post','/admin/rewards/revenue/evidence','reward_revenue_evidence','RewardProposalResult',False),('post','/admin/rewards/revenue/evidence/{id}/approve','reward_reason',None,False),('post','/admin/rewards/revenue/reconcile','reward_revenue_day','RewardRevenueResult',False)])

routes.extend([('get','/admin/rewards/calculations/{id}',None,None,False),('get','/admin/rewards/policy/history',None,'RewardItems',False)])
