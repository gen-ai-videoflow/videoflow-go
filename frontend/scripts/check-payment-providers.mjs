import assert from 'node:assert/strict';
import {paymentProviders, paymentEndpoint} from '../src/lib/cloud/payment-providers.mjs';
assert.deepEqual(paymentProviders({stripe_enabled:true,cryptorails_enabled:true}),['stripe','cryptorails']);
assert.deepEqual(paymentProviders({stripe_enabled:false,cryptorails_enabled:false}),[]);
assert.equal(paymentEndpoint('stripe','credits'),'/billing/checkout');
assert.equal(paymentEndpoint('stripe','premium'),'/premium/checkout');
assert.equal(paymentEndpoint('cryptorails','credits'),'/payments/invoices');
assert.throws(()=>paymentEndpoint('invalid','premium'));
console.log('Payment provider selection assertions passed');
