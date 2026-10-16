export type PaymentProvider = 'stripe' | 'cryptorails';
export function paymentProviders(options?:{stripe_enabled?:boolean;cryptorails_enabled?:boolean}):PaymentProvider[];
export function paymentEndpoint(provider:PaymentProvider,purpose:'credits'|'premium'):string;
