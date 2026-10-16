<script lang="ts">
 import { CreditCard, Wallet } from '@lucide/svelte';
 import { paymentProviders, type PaymentProvider } from './payment-providers.mjs';
 let {options,value=$bindable('stripe')}:{options:any;value?:PaymentProvider}=$props();
 const available=$derived(paymentProviders(options));
 $effect(()=>{if(available.length&&!available.includes(value))value=available[0];});
</script>
{#if available.length}
<fieldset class="payment-choice"><legend>Payment method</legend><div class="payment-choice-options">
{#each available as provider}<label class:chosen={value===provider}><input type="radio" name="payment-provider" value={provider} bind:group={value}/>{#if provider==='stripe'}<CreditCard size={19}/><span><strong>Card</strong><small>Secure checkout with Stripe</small></span>{:else}<Wallet size={19}/><span><strong>Crypto</strong><small>Token transfer with CryptoRails</small></span>{/if}</label>{/each}
</div></fieldset>
{/if}
<style>
.payment-choice{border:0;padding:0;margin:22px 0;min-width:0}.payment-choice legend{font-size:13px;font-weight:600;margin-bottom:10px}.payment-choice-options{display:flex;gap:10px;flex-wrap:wrap}.payment-choice label{display:flex;align-items:center;gap:11px;border:1px solid var(--border);padding:13px 16px;border-radius:var(--radius);cursor:pointer;min-width:220px;flex:1}.payment-choice label.chosen{border-color:var(--primary);background:var(--muted)}.payment-choice input{width:16px;height:16px;accent-color:var(--primary)}.payment-choice strong,.payment-choice small{display:block}.payment-choice strong{font-size:13px;font-weight:600}.payment-choice small{font-size:12px;color:var(--muted-foreground);margin-top:3px}.payment-choice label:focus-within{outline:2px solid var(--ring);outline-offset:2px}
</style>
