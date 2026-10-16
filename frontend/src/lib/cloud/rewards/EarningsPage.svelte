<script lang="ts">
 import {onMount} from 'svelte';
 import CreatorAnalytics from '../CreatorAnalytics.svelte';
 import * as Tabs from '$lib/components/ui/tabs';
 import {Button} from '$lib/components/ui/button';
 import {Input} from '$lib/components/ui/input';
 import {AlertCircle,ArrowUpRight,Clock3,RefreshCw,ShieldCheck} from '@lucide/svelte';
 import {request} from '../api';
 import type {RewardBalance,RewardProgram,RewardVideo} from './client';
 let {user}:{user:any}=$props();
 let summary:RewardBalance|null=$state(null), program:RewardProgram|null=$state(null), videos:RewardVideo[]=$state([]);
 let payouts:any=$state(null), entries:any[]=$state([]), error=$state(''), busy=$state(false), loading=$state(true), offset=$state(0), tab=$state('overview');
 let schedules=$state<any[]>([]),payments=$state<any[]>([]),analyticsOpen=$state(false),payoutOpen=$state(false),rulesOpen=$state(false),historyLoaded=$state(false),historyLoading=$state(false),videosLoading=$state(false);
 let country=$state(''), accepted=$state(false), selected:RewardVideo|null=$state(null);
 let zh=$state(false);
 const t=(en:string,cn:string)=>zh?cn:en;
 const money=(n:number|null|undefined)=>n==null?'—':new Intl.NumberFormat(zh?'zh-CN':'en-US',{style:'currency',currency:'USD'}).format(n/100);
 const date=(v:string|null|undefined)=>v?new Date(v).toLocaleString(zh?'zh-CN':'en-US',{dateStyle:'medium',timeStyle:'short'}):'—';
 const reasons:Record<string,[string,string]>={not_published:['Publish your video first','请先发布视频'],window_ended:['Earning window ended','收益窗口已结束'],not_public:['Video is not publicly published','视频未公开发布'],program_unavailable_or_not_enrolled:['Program unavailable or enrollment needed','计划暂不可用或尚未加入'],evidence_review_required:['Qualified evidence needs review','合资格数据有待审核']};
 async function load(){loading=true;error='';try{
  const [s,p,a]=await Promise.all([request('/creator/rewards/summary'),request('/creator/rewards/program'),request('/creator/payout-account')]);
  summary=s;program=p;payouts=a;country=a.country||'';if(payoutOpen)await loadHistory();
 }catch(e){error=e instanceof Error?e.message:t('Could not load earnings. Try again.','无法加载收益，请重试。');}finally{loading=false}}
 async function loadVideos(){videosLoading=true;try{const result=await request('/creator/rewards/videos?offset='+offset);videos=result.videos}catch(e){error=String(e)}finally{videosLoading=false}}
 async function loadHistory(){if(historyLoading)return;historyLoading=true;try{const [e,h]=await Promise.all([request('/creator/earnings'),request('/creator/rewards/payouts')]);entries=e.entries||[];payments=h.payouts||[];historyLoaded=true}catch(e){error=String(e)}finally{historyLoading=false}}
 $effect(()=>{if(tab==='videos')void loadVideos()});
 async function detail(video:RewardVideo){
  if(selected?.video_id===video.video_id){selected=null;return}selected=video;schedules=[];
  try{const result=await request('/creator/rewards/videos/'+encodeURIComponent(video.video_id));if(selected?.video_id===video.video_id)schedules=result.allocations||[]}catch(e){error=String(e)}
 }
 async function enroll(){busy=true;error='';try{await request('/creator/rewards/enroll',{policy_version:program?.policy_version,accepted});await load()}catch(e){error=String(e)}finally{busy=false}}
 async function setup(){busy=true;error='';try{const result=await request('/creator/payout-account',{country:country.toUpperCase()});window.location.assign(result.url)}catch(e){error=String(e)}finally{busy=false}}
 onMount(()=>{zh=document.documentElement.lang.startsWith('zh');const target=new URL(location.href).searchParams.get('tab');if(target==='videos')tab='videos';if(target==='payouts')payoutOpen=true;if(target==='program')rulesOpen=true;void load()});
</script>

<div class="earnings-heading"><div><h1>{t('Your earnings','我的收益')}</h1><p>{t('See what you’ve earned and what happens next.','查看收益和下一步操作。')}</p></div><Button variant="outline" disabled={loading} onclick={()=>{void load();if(tab==='videos')void loadVideos()}}><RefreshCw size={15}/>{t('Refresh','刷新')}</Button></div>
{#if error}<div class="notice error" role="alert"><AlertCircle size={19}/><div><strong>{t('Earnings could not be updated','收益暂时无法更新')}</strong><p>{error}</p><Button variant="outline" onclick={load}>{t('Try again','重试')}</Button></div></div>{/if}
{#if loading&&!summary}<p role="status" class="quiet">{t('Loading your earnings…','正在加载收益…')}</p>
{:else if summary}
 {#if !program?.enabled}<div class="notice"><ShieldCheck size={20}/><div><strong>{t('Creator rewards are not active yet','创作者奖励尚未启用')}</strong><p>{t('You can keep creating, publishing and watching. Existing earnings remain visible. No payment per view is guaranteed.','你仍可创作、发布和观看视频。已有收益仍会显示，每次观看不保证产生奖励。')}</p></div></div>{/if}
 <Tabs.Root bind:value={tab}>
  <Tabs.List class="reward-tabs" aria-label={t('Earnings sections','收益栏目')}>
   <Tabs.Trigger value="overview">{t('Earnings','收益')}</Tabs.Trigger><Tabs.Trigger value="videos">{t('Videos','视频')}</Tabs.Trigger>
  </Tabs.List>
  <Tabs.Content value="overview">
   <section class="earnings-balance">
    <h2>{t('Available to receive','可收取收益')}</h2>
    <strong>{summary.held?'—':money(summary.available_cents)}</strong>
    <p>{summary.held?t('Payments are paused for account review. Your earnings are preserved.','付款因账户审核暂缓，已有收益仍被保留。'):t('Payments follow account verification and the payout schedule.','付款须完成账户验证，并按排期处理。')}</p>
    {#if summary.confirmed_unvested_cents>0}<p class="unlocking"><b>{money(summary.confirmed_unvested_cents)}</b> {t('is unlocking gradually over 30 days.','正在 30 天内逐步解锁。')}</p>{/if}
    {#if summary.paid_cents>0}<p class="quiet">{t('Transferred so far','累计已转款')}: {money(summary.paid_cents)}</p>{/if}
   </section>
   <div class="payout-status"><Clock3 size={21}/><div><h2>{t('Your next payment','下一次付款')}</h2><p>{!payouts?.cash_payouts_enabled?t('Payouts are not available yet.','付款功能暂未开放。'):t('Approved monthly payout runs. No transfer is scheduled until verified and approved.','按月审核付款，完成验证与审批后才会安排转账。')}</p><Button variant="outline" onclick={()=>{payoutOpen=true;requestAnimationFrame(()=>document.getElementById('payout-settings')?.scrollIntoView({block:'start'}))}}>{t('Manage payouts','管理收款')}</Button></div></div>
   {#if program?.enabled&&!program.enrolled}<Button variant="outline" onclick={()=>rulesOpen=true}>{t('Learn about creator rewards','了解创作者奖励')}</Button>{/if}
   <p class="quiet">{t('Updated','更新时间')} {date(summary.calculated_at)} · {t('Generation credits are separate from cash earnings.','生成点数与现金收益分开计算。')}</p>
   <details class="secondary-detail" ontoggle={(e)=>analyticsOpen=e.currentTarget.open}><summary>{t('Audience analytics','受众分析')}</summary>{#if analyticsOpen}<CreatorAnalytics/>{/if}</details>
   {#if summary.legacy_pending_cents!==0}<p class="quiet">{t('Legacy pending earnings','旧版待处理收益')}: {money(summary.legacy_pending_cents)}. {t('Existing settlement rules continue to apply.','继续适用原有结算规则。')}</p>{/if}
   {#if summary.estimated_additional_cents!==null}<details class="secondary-detail"><summary>{t('Estimated rewards','预估奖励')}</summary><p>{money(summary.estimated_additional_cents)} · {t('An estimate only. It can change before confirmation.','仅为预估，确认前可能变动。')}</p></details>{/if}
<details id="payout-settings" class="secondary-detail" bind:open={payoutOpen} ontoggle={(e)=>{if(e.currentTarget.open&&!historyLoaded)void loadHistory()}}><summary>{t('Payout settings & history','付款设置与记录')}</summary>
   <section class="setup"><h2>{t('Payout account','收款账户')}</h2><p>{payouts?.configured?t('Your account is connected. Complete provider verification if requested.','账户已关联，如有提示，请完成支付服务商验证。'):t('Connect an eligible account when onboarding becomes available.','收款设置开放后，请关联符合条件的账户。')}</p>
    {#if payouts?.configured}<p class="quiet">{t('Country','国家或地区')}: {payouts.country} · {payouts.payouts_enabled?t('Provider verified','服务商已验证'):t('Verification incomplete','尚未完成验证')}</p>{/if}
    <form onsubmit={(e)=>{e.preventDefault();void setup()}}><label for="reward-country">{t('Legal residence country code','法定居住国家或地区代码')}</label><Input id="reward-country" bind:value={country} placeholder="US" pattern="[A-Za-z]{2}" maxlength={2} required disabled={payouts?.configured}/><p class="quiet">{t('Use the two-letter country code. Provider availability is checked before setup.','输入两个字母的国家或地区代码，设置前会检查服务商是否支持。')}</p><Button disabled={busy||!payouts?.onboarding_enabled||!user?.verified} type="submit">{payouts?.configured?t('Continue verification','继续验证'):t('Set up payout account','设置收款账户')}<ArrowUpRight size={15}/></Button></form>
    {#if !user?.verified}<a href="/settings">{t('Verify your email in settings','前往设置验证邮箱')}</a>{/if}
   </section>
   {#if historyLoaded}<h2>{t('Payment history','付款记录')}</h2>
   {#each payments as payment}<div class="activity"><span><strong>{payment.status==='paid'?t('Transferred to connected account','已转至关联账户'):payment.status}</strong><small>{payment.id}</small></span><span>{money(payment.amount_cents)}</span></div>{:else}<p class="quiet">{t('No payment batches yet.','暂时没有付款批次。')}</p>{/each}
   <p class="quiet">{t('A transfer to your connected account does not confirm arrival at your bank.','转至关联账户并不代表银行账户已到账。')}</p>
   <div class="section-head"><h2>{t('Earnings activity','收益记录')}</h2><span class="quiet">{t('Paid to date','累计已付款')} {money(summary.paid_cents)}</span></div>
   {#each entries as entry}<div class="activity"><span><strong>{entry.description}</strong><small>{date(entry.created_at)} · {entry.status}</small></span><span>{money(entry.amount_cents)}</span></div>{:else}<p class="empty">{t('Confirmed earnings and adjustments will appear here.','已确认收益及调整记录会显示在这里。')}</p>{/each}{:else}<p role="status">{t('Loading payment history…','正在加载付款记录…')}</p>{/if}
</details>
<details class="secondary-detail" bind:open={rulesOpen}><summary>{t('How rewards work','奖励如何运作')}</summary>
   <section class="program"><h2>{t('Create, publish, then track your rewards','创作、发布，再追踪奖励')}</h2><ol><li>{t('Eligible original videos have a 30-day earning window after first publication.','符合条件的原创视频从首次发布起有 30 天收益窗口。')}</li><li>{t('Rewards depend on qualified performance and available funded budgets. Exact scoring weights are private.','奖励取决于合资格表现与已获得资金支持的预算，具体评分权重不公开。')}</li><li>{t('Daily confirmed batches vest gradually over 30 days. Estimates can change and are not money owed.','每日确认批次在 30 天内逐步解锁。预估金额可能变化，并非已承诺付款。')}</li><li>{t('Rewards may exceed your spending, but no earnings or recovery of costs are guaranteed.','奖励可能超过你的支出，但不保证收益或收回成本。')}</li></ol><p>{t('Eligible platform revenue may fund rewards. Generation credits, promotional credits and cash earnings remain separate. Support-credit milestones are not available yet.','符合条件的平台收入可用于奖励。生成点数、赠送点数和现金收益分开计算，创作里程碑点数奖励尚未开放。')}</p>
    {#if program?.enrolled}<p>{t('You have joined this program version.','你已加入当前版本的奖励计划。')}</p>{:else if program?.enabled}<label class="accept"><input type="checkbox" bind:checked={accepted}/>{t('I understand the variable rewards, earning window and vesting rules.','我已了解浮动奖励、收益窗口及逐步解锁规则。')}</label><Button disabled={!accepted||busy||!user?.verified} onclick={enroll}>{t('Join creator rewards','加入创作者奖励')}</Button>{:else}<p class="quiet">{t('Enrollment opens when the published program policy and funding are approved.','计划规则与资金获得批准后，才会开放加入。')}</p>{/if}
   </section>
</details>
  </Tabs.Content>
  <Tabs.Content value="videos">
   <div class="section-head"><h2>{t('Your videos','我的视频')}</h2><span class="quiet">{t('Select a video for details','选择视频查看详情')}</span></div>
   {#if videosLoading}<p class="quiet" role="status">{t('Loading videos…','正在加载视频…')}</p>{:else}{#each videos as video}<button class="video-row" onclick={()=>detail(video)} aria-expanded={selected?.video_id===video.video_id}><span><strong>{video.title}</strong><small>{reasons[video.reason_code]?.[zh?1:0]||video.status}</small></span><span>{date(video.window_ends_at)}</span></button>
    {#if selected?.video_id===video.video_id}<div class="video-detail"><p>{t('First published','首次发布')}: {date(video.first_published_at)}</p><p>{t('Window ends','窗口结束')}: {date(video.window_ends_at)}</p><p>{t('Audience views are actual playback metrics. They are not a promised cash rate. Republishing does not restart the window.','观看量反映实际播放数据，不代表固定现金奖励。重新发布不会重置收益窗口。')}</p>{#each schedules as schedule}<p>{t('Confirmed','已确认')}: {money(schedule.cents)} · {t('Vested','已解锁')}: {money(schedule.vested_cents)}<br/>{t('Fully vested','完全解锁')}: {date(schedule.fully_vested_at)}</p>{:else}<p>{t('No confirmed reward batches for this video.','此视频尚无已确认奖励批次。')}</p>{/each}</div>{/if}
   {:else}<div class="empty"><h3>{t('Your videos will appear here','你的视频会显示在这里')}</h3><p>{t('Publish an original upload or a finished Studio video. Buying credits is not a reward requirement.','发布原创上传视频或 Studio 成片，参与奖励不要求购买点数。')}</p><Button href="/render" variant="outline">{t('Open your content','查看我的内容')}</Button></div>{/each}{/if}
   {#if videos.length>0||offset>0}<div class="pagination"><Button variant="outline" disabled={offset===0||videosLoading} onclick={()=>{offset=Math.max(0,offset-25);void loadVideos()}}>{t('Previous','上一页')}</Button><Button variant="outline" disabled={videos.length<25||videosLoading} onclick={()=>{offset+=25;void loadVideos()}}>{t('Next','下一页')}</Button></div>{/if}
  </Tabs.Content>


 </Tabs.Root>
{/if}

<style>
 .earnings-heading{display:flex;justify-content:space-between;align-items:start;gap:24px;margin-bottom:28px}.earnings-heading h1{font-size:1.5rem;font-weight:650;letter-spacing:-.03em;margin:0 0 8px}.earnings-heading p,.quiet{color:var(--muted-foreground);font-size:.875rem;line-height:1.6}.notice{display:flex;gap:14px;padding:18px;border:1px solid var(--border);border-radius:10px;margin-bottom:24px;background:var(--muted)}.notice :global(svg){flex-shrink:0}.notice p{font-size:.875rem;line-height:1.6;margin:5px 0 0}.notice.error{color:var(--destructive)}h2{font-size:.875rem;font-weight:600;margin:0 0 12px}.earnings-balance{padding:32px 0 28px;border-bottom:1px solid var(--border)}.earnings-balance>strong{display:block;font-size:2.5rem;font-weight:600;letter-spacing:-.03em;font-variant-numeric:tabular-nums;margin:12px 0}.earnings-balance p{color:var(--muted-foreground);font-size:.875rem;line-height:1.7}.earnings-balance .unlocking{margin-top:20px}.unlocking b{color:var(--foreground)}.secondary-detail{padding:18px 0;border-top:1px solid var(--border);font-size:.875rem;line-height:1.8}.secondary-detail summary{cursor:pointer;font-weight:500}.secondary-detail[open] summary{margin-bottom:16px}.secondary-detail summary:focus-visible{outline:2px solid var(--ring);outline-offset:4px}.payout-status{display:flex;gap:16px;padding:24px 0}.payout-status p,.program p,.program li,.setup p{font-size:.875rem;line-height:1.8;max-width:70ch;color:var(--muted-foreground)}.payout-status :global(svg){flex-shrink:0}.section-head{display:flex;justify-content:space-between;align-items:center;gap:16px;margin:28px 0 12px}.video-row,.activity{display:flex;justify-content:space-between;align-items:center;gap:24px;width:100%;padding:18px 0;border:0;border-bottom:1px solid var(--border);text-align:left;background:transparent;color:inherit;font-size:.875rem}.video-row{cursor:pointer}.video-row:hover{background:var(--muted)}.video-row small,.activity small{display:block;color:var(--muted-foreground);margin-top:6px}.video-row>span:last-child{color:var(--muted-foreground);font-size:.75rem}.video-detail{background:var(--muted);padding:18px;font-size:.875rem;line-height:1.8}.empty{padding:40px 0;color:var(--muted-foreground);line-height:1.8}.empty h3{color:var(--foreground);font-weight:600}.pagination{display:flex;justify-content:flex-end;gap:12px;margin-top:24px}.setup,.program{padding:28px 0}.setup form{display:grid;gap:12px;max-width:380px;margin:24px 0}.setup label{font-size:.875rem}.program ol{padding-left:24px}.program li{padding-left:6px;margin:12px 0}.accept{display:flex;gap:12px;align-items:start;margin:24px 0;font-size:.875rem;line-height:1.7}.accept input{margin-top:6px;accent-color:var(--primary)}@media(max-width:680px){.earnings-heading{gap:12px}.earnings-heading h1{font-size:1.5rem}.video-row,.section-head{align-items:start;flex-direction:column;gap:8px}}
</style>
