<script lang="ts">
 import {request} from '../api';
 let {zh=false}:{zh?:boolean}=$props();
 type Item={cents?:number;approved_at?:string;vesting_seconds?:number;type?:string;id:string;title?:string;criteria?:string;enabled?:boolean;budget_available?:boolean;grant_credits?:number;expiry_days?:number;creator_cap?:number;status?:string;reason?:string;available?:number;reserved?:number;expires_at?:string;subject?:string;resolution?:string;message?:string;read_at?:string|null};
 let items=$state<Item[]>([]),appeals=$state<Item[]>([]),notifications=$state<Item[]>([]),corrections=$state<Item[]>([]);
 let loaded=$state(false),loading=$state(false),busy=$state(false),error=$state(''),evidence=$state<Record<string,string>>({}),subject=$state('eligibility'),appealEvidence=$state('');
 const t=(en:string,cn:string)=>zh?cn:en;
 const date=(v?:string)=>v?new Date(v).toLocaleDateString(zh?'zh-CN':'en-US'):'—';
 const status=(v?:string)=>({pending:t('Under review','审核中'),approved:t('Approved','已批准'),rejected:t('Declined','未批准'),open:t('Awaiting review','等待审核'),upheld:t('Upheld','申诉成立'),declined:t('Declined','申诉未成立')}[v||'']||v);
 async function load(){if(loading)return;loading=true;error='';try{const [s,a,n,c]=await Promise.all([request('/creator/rewards/support'),request('/creator/rewards/appeals'),request('/creator/rewards/notifications'),request('/creator/rewards/corrections')]);corrections=c.items;items=s.items;appeals=a.items;notifications=n.items;loaded=true}catch(e){error=e instanceof Error?e.message:t('Unable to load details.','无法加载详情。')}finally{loading=false}}
 async function mutate(path:string,body:object){busy=true;error='';try{await request(path,body);await load();return true}catch(e){error=e instanceof Error?e.message:t('Unable to submit. Try again.','无法提交，请重试。');return false}finally{busy=false}}
</script>
<details class="support-details" ontoggle={(event)=>{if(event.currentTarget.open&&!loaded)void load()}}>
 <summary>{t('Creator support, reviews & updates','创作者支持、复核与通知')}</summary>
 <p>{t('Support credits pay for generation services. They expire and cannot be withdrawn or exchanged for cash. Buying credits does not unlock support or earnings.','支持点数仅用于生成服务，有效期届满后失效，不可提现或兑换现金。购买点数不会解锁支持或收益。')}</p>
 {#if error}<p role="alert">{error}</p>{/if}
 {#if loading}<p role="status">{t('Loading details…','正在加载详情…')}</p>{:else if loaded}
  <h3>{t('Promotional generation credits','推广生成点数')}</h3>
  {#each items.filter(i=>i.type==='credit') as credit (credit.id)}<p>{credit.available} {t('available','可用')} · {credit.reserved} {t('reserved for generation','已预留用于生成')} · {t('Expires','到期日期')} {date(credit.expires_at)}</p>{:else}<p>{t('No promotional credits. Purchased credits appear in your generation balance.','暂无推广点数。已购买点数显示在生成余额中。')}</p>{/each}
  <h3>{t('Published support milestones','已公布的支持里程碑')}</h3>
  {#each items.filter(i=>i.type==='program') as program (program.id)}
   <section><h4>{program.title}</h4><p>{program.criteria}</p><p>{program.grant_credits} {t('generation credits','生成点数')} · {t('Valid for','有效期')} {program.expiry_days} {t('days after approval','天，自批准之日起')} · {t('Maximum grants per creator','每位创作者最多获批次数')}: {program.creator_cap}</p>
    {#if program.enabled&&program.budget_available}<form onsubmit={async e=>{e.preventDefault();if(await mutate('/creator/rewards/support/'+encodeURIComponent(program.id)+'/claims',{evidence:evidence[program.id]||''}))evidence[program.id]=''}}><label>{t('Evidence of original work','原创作品证明')}<textarea bind:value={evidence[program.id]} required maxlength={2000}></textarea></label><button disabled={busy}>{t('Request review','申请审核')}</button></form>{:else}<p>{t('Requests are currently unavailable.','目前暂不接受申请。')}</p>{/if}
   </section>
  {:else}<p>{t('No support milestones are currently published.','目前尚未公布支持里程碑。')}</p>{/each}
  {#each items.filter(i=>i.type==='claim') as claim (claim.id)}<p>{t('Support request','支持申请')}: {status(claim.status)}{claim.reason?' · '+claim.reason:''}</p>{/each}
  <h3>{t('Ask for a reward review','申请奖励复核')}</h3>
  <p>{t('A review records your evidence and the decision. Any financial correction needs a separate funded approval.','复核会记录您的证据和处理结果。任何财务更正均须单独审核并落实资金。')}</p>
  <form onsubmit={async e=>{e.preventDefault();if(await mutate('/creator/rewards/appeals',{subject,evidence:appealEvidence}))appealEvidence=''}}>
   <label>{t('Topic','主题')}<select bind:value={subject}><option value="eligibility">{t('Eligibility','资格')}</option><option value="hold">{t('Account hold','账户限制')}</option><option value="support">{t('Support credits','支持点数')}</option><option value="payout">{t('Payment','付款')}</option></select></label>
   <label>{t('What should we review? Include supporting references.','请说明复核事项并附上证明资料。')}<textarea bind:value={appealEvidence} required maxlength={4000}></textarea></label><button disabled={busy}>{t('Submit review request','提交复核申请')}</button>
  </form>
  {#each appeals as appeal (appeal.id)}<p>{status(appeal.status)}{appeal.resolution?' · '+appeal.resolution:''}</p>{/each}
  {#if corrections.length}<h3>{t('Approved reward corrections','已批准的奖励更正')}</h3>{#each corrections as correction (correction.id)}<p>{new Intl.NumberFormat(zh?'zh-CN':'en-US',{style:'currency',currency:'USD',signDisplay:'always'}).format((correction.cents||0)/100)} · {correction.reason} · {date(correction.approved_at)}{#if (correction.cents||0)>0} · {t('Unlocks gradually over','逐步解锁天数')} {(correction.vesting_seconds||0)/86400} {t('days','天')}{/if}</p>{/each}{/if}
  <h3>{t('Updates','通知')}</h3>
  {#each notifications as notification (notification.id)}<article><p>{notification.message}</p>{#if !notification.read_at}<button disabled={busy} onclick={()=>mutate('/creator/rewards/notifications/'+encodeURIComponent(notification.id)+'/read',{})}>{t('Mark as read','标记为已读')}</button>{/if}</article>{:else}<p>{t('No reward updates yet.','暂无奖励通知。')}</p>{/each}
 {/if}
</details>
<style>
 .support-details{border-top:1px solid var(--border,#ccc);padding:1.25rem 0;margin-top:1rem}summary{cursor:pointer;font-weight:600}h3{font-size:1rem;margin:1.5rem 0 .5rem}h4{margin:.5rem 0}p{line-height:1.6;margin:.65rem 0}section,article{padding:.75rem 0;border-bottom:1px solid var(--border,#ccc)}form{display:grid;gap:.75rem;max-width:40rem}label{display:grid;gap:.35rem}textarea,select,button{font:inherit;border:1px solid var(--border,#888);border-radius:.4rem;background:var(--background,transparent);color:inherit;padding:.6rem}textarea{min-height:6rem;width:100%;box-sizing:border-box}button{justify-self:start;cursor:pointer}button:disabled{opacity:.55;cursor:wait}:focus-visible{outline:2px solid currentColor;outline-offset:3px}[role=alert]{color:var(--destructive,#b42318)}
</style>
