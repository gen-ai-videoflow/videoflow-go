export function paymentProviders(options) {
 return ['stripe','cryptorails'].filter(provider => options?.[`${provider}_enabled`] === true);
}
export function paymentEndpoint(provider,purpose) {
 if(!['credits','premium'].includes(purpose))throw new Error('Choose a supported product.');
 if(provider==='cryptorails')return '/payments/invoices';
 if(provider==='stripe')return purpose==='credits'?'/billing/checkout':'/premium/checkout';
 throw new Error('Choose an available payment method.');
}
