/** Deposit instructions are available only for an actionable server invoice. */
export function canDeposit(invoice: {status:string;address:string}): boolean {
  return Boolean(invoice.address?.trim()) && ['pending','processing'].includes(invoice.status);
}
export function blockedPayment(invoice: {status:string;address:string}): {title:string;message:string} {
  if (invoice.status === 'failed') return {title:'Payment was not completed',message:'Keep this invoice ID and any transaction hash. Contact support before sending another payment; a failed status does not establish whether a transfer can be recovered.'};
  if (invoice.status === 'expired') return {title:'Payment instructions are no longer available',message:'Refresh this invoice and contact support with its ID before paying again. If you already transferred funds, keep the transaction hash for reconciliation.'};
  return {title:'Payment details require review',message:'Contact support with this invoice ID and any transaction hash. Do not send funds while instructions are unavailable or under review. No automatic recovery or refund is promised.'};
}
export function restoredInvoiceAttempt(value: unknown): {key:string;id?:string}|undefined {
  if (!value || typeof value !== 'object') return;
  const attempt=value as Record<string,unknown>;
  if (typeof attempt.key!=='string' || !/^[A-Za-z0-9_-]{8,200}$/.test(attempt.key)) return;
  if (attempt.id!==undefined && (typeof attempt.id!=='string' || !attempt.id || attempt.id.length>200)) return;
  return {key:attempt.key,...(typeof attempt.id==='string'?{id:attempt.id}:{})};
}
