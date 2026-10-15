#!/usr/bin/env python3
"""TEST ONLY CryptoRails gateway; no blockchain, real wallets, or real money.

Loopback by default. For a private Docker network explicitly pass
--host 0.0.0.0 --private-docker and publish ports on host 127.0.0.1 only.
All gateway AND test-control routes require the mock x-tenant-api-key.
"""
import argparse
import datetime as dt
from decimal import Decimal, InvalidOperation, ROUND_CEILING, getcontext
import hashlib
import hmac
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import ipaddress
import json
import os
from pathlib import Path
import threading
import urllib.error
import urllib.parse
import urllib.request
import uuid

getcontext().prec = 100
UNIT = Decimal('0.000000000000000001')


def amount(raw):
    text = str(raw)
    value = Decimal(text)
    if not value.is_finite() or value < 0 or value.as_tuple().exponent < -18:
        raise ValueError('invalid exact decimal amount')
    return value


def decimal(raw):
    return format(amount(raw).quantize(UNIT), 'f')


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class Gateway:
    def __init__(self, config):
        self.config = config
        self.lock = threading.RLock()
        self.path = Path(config.state_dir)
        self.path.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.file = self.path / 'gateway-state.json'
        self.state = json.loads(self.file.read_text()) if self.file.exists() else {
            'mock_test_only': True, 'addresses': {}, 'transactions': {}, 'fail_delivery': False}
        self.save()

    def save(self):
        temporary = self.path / 'gateway-state.tmp'
        temporary.write_text(json.dumps(self.state, indent=2))
        temporary.chmod(0o600)
        temporary.replace(self.file)

    def fee(self, gross):
        return (gross * Decimal(self.config.fee_bps) / Decimal(10000) + amount(self.config.fixed_fee)).quantize(UNIT, rounding=ROUND_CEILING)

    def deliver(self, reference):
        # Delivery uses the preserved exact original bytes and original HMAC.
        # Deliberately no automatic retry is scheduled after any failure.
        with self.lock:
            tx = self.state['transactions'].get(reference)
            if not tx or not tx.get('original_raw_body'):
                raise ValueError('confirmed/failed original event is not available')
            raw = tx['original_raw_body'].encode()
            signature = tx['original_signature']
            if self.state['fail_delivery']:
                result = {'status': 'failed', 'http_status': 503, 'simulated_failure': True,
                          'detail': 'TEST CONTROL: callback deliberately withheld; no automatic retry'}
            else:
                request = urllib.request.Request(self.config.callback, data=raw, method='POST', headers={
                    'Content-Type': 'application/json', 'x-cr-webhook-signature': signature,
                    'User-Agent': 'CR-Webhooks/1.0-MOCK-TEST-ONLY'})
                try:
                    with urllib.request.build_opener(NoRedirect).open(request, timeout=15) as response:
                        status = response.status
                        result = {'status': 'delivered' if 200 <= status < 300 else 'failed',
                                  'http_status': status, 'response': response.read(4096).decode(errors='replace')}
                except urllib.error.HTTPError as error:
                    result = {'status': 'failed', 'http_status': error.code,
                              'response': error.read(4096).decode(errors='replace')}
                    error.close()
                except (urllib.error.URLError, TimeoutError, OSError):
                    result = {'status': 'failed', 'http_status': None, 'detail': 'callback unavailable'}
            tx['deliveries'].append(result)
            self.save()
            return result


class Handler(BaseHTTPRequestHandler):
    server_version = 'CryptoRailsMOCK-TEST-ONLY/1.0'

    def log_message(self, fmt, *args):
        # Route-only logs: never log the API header, body, or original signatures.
        print(f'MOCK TEST ONLY {self.command} {urllib.parse.urlsplit(self.path).path} {args[1] if len(args)>1 else ""}', flush=True)

    def reply(self, status, value):
        raw = json.dumps(value, separators=(',', ':')).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Cache-Control', 'no-store')
        self.send_header('Content-Length', str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def error(self, status, message):
        self.reply(status, {'statusCode': status, 'message': message, 'mock_test_only': True})

    def authorized(self):
        supplied = self.headers.get('x-tenant-api-key', '')
        if not hmac.compare_digest(supplied, self.server.gateway.config.api_key):
            self.error(401, 'Unauthorized')
            return False
        return True

    def body(self):
        length = int(self.headers.get('Content-Length', '0'))
        if length < 0 or length > 65536:
            raise ValueError('body too large')
        value = json.loads(self.rfile.read(length) or b'{}', parse_float=Decimal)
        if not isinstance(value, dict):
            raise ValueError('object body required')
        return value

    def do_GET(self):
        if not self.authorized():
            return
        gateway = self.server.gateway
        cfg = gateway.config
        parts = urllib.parse.urlsplit(self.path)
        query = urllib.parse.parse_qs(parts.query)
        one = lambda key: query.get(key, [''])[0]
        with gateway.lock:
            if parts.path == '/v1/api/tenant-token-statuses':
                matches = one('tokenAddress').lower() == cfg.token_address.lower() and one('blockchainName') == cfg.network
                data = [{'id': 'mock-policy', 'isActive': True,
                         'depositProviderFeeFixed': decimal(cfg.fixed_fee),
                         'depositProviderFeePer': format(Decimal(cfg.fee_bps) / 100, 'f'),
                         'tenantConsolidationAddress': cfg.treasury,
                         'tokenConfig': {'name': cfg.token, 'decimal': cfg.decimals,
                                         'tokenContractAddress': cfg.token_address,
                                         'blockchainName': cfg.network, 'isActive': True, 'type': 'ERC20'}}] if matches else []
                self.reply(200, {'data': data, 'meta': {'hasNextPage': False}, 'mock_test_only': True})
            elif parts.path == '/v1/api/address':
                record = gateway.state['addresses'].get(one('address'))
                if not record or record['reference'] != one('reference') or one('blockchainName') not in ('', cfg.network):
                    self.error(404, 'error.addressNotFound')
                else:
                    self.reply(200, {key: value for key, value in record.items() if key != 'balance'})
            elif parts.path == '/v1/api/transaction':
                record = gateway.state['transactions'].get(one('reference'))
                if not record:
                    self.error(404, 'error.transactionNotFound')
                else:
                    self.reply(200, {key: value for key, value in record.items() if key not in ('original_raw_body', 'original_signature')})
            elif parts.path == '/__test__/state':
                self.reply(200, gateway.state)
            elif parts.path.startswith('/__test__/events/'):
                reference = urllib.parse.unquote(parts.path.removeprefix('/__test__/events/'))
                record = gateway.state['transactions'].get(reference, {})
                if not record.get('original_raw_body'):
                    self.error(404, 'test.originalEventNotFound')
                else:
                    self.reply(200, {'raw_body': record['original_raw_body'], 'signature': record['original_signature'],
                                     'deliveries': record['deliveries'], 'mock_test_only': True})
            else:
                self.error(404, 'test.routeNotFound')

    def do_POST(self):
        if not self.authorized():
            return
        try:
            body = self.body()
            self.post(urllib.parse.urlsplit(self.path).path, body)
        except (ValueError, InvalidOperation, KeyError, TypeError):
            self.error(422, 'test.invalidRequest')

    def post(self, route, body):
        gateway = self.server.gateway
        cfg = gateway.config
        if route == '/__test__/deliver':
            self.reply(200, gateway.deliver(body['reference']))
            return
        with gateway.lock:
            addresses = gateway.state['addresses']
            transactions = gateway.state['transactions']
            if route == '/v1/api/create-address':
                if body.get('vmType') != cfg.vm_type:
                    self.error(400, 'error.inactiveVmType')
                    return
                reference = body['reference']
                if not isinstance(reference, str) or not reference:
                    raise ValueError('reference required')
                if any(value['reference'] == reference for value in addresses.values()):
                    self.error(409, 'error.tenantReferenceExists')
                    return
                address = '0x' + hashlib.sha256(('MOCK ONLY:' + reference).encode()).hexdigest()[:40]
                record = {'id': str(uuid.uuid4()), 'address': address, 'reference': reference,
                          'name': body.get('name'), 'isWithdrawalAddress': False,
                          'vmType': {'id': 'mock-vm', 'type': cfg.vm_type, 'isActive': True},
                          'balance': '0.000000000000000000', 'mock_test_only': True}
                addresses[address] = record
                gateway.save()
                self.reply(201, {key: value for key, value in record.items() if key != 'balance'})
            elif route == '/v1/api/consolidate':
                reference = body['transactionReference']
                if reference in transactions:
                    self.error(409, 'error.transactionAlreadyExist')
                    return
                record = addresses.get(body['address'])
                if not record:
                    self.error(404, 'error.addressNotFound')
                    return
                if body.get('blockchainName') != cfg.network or body.get('tokenAddress', '').lower() != cfg.token_address.lower():
                    self.error(400, 'error.inactiveToken')
                    return
                gross = amount(body['amount'])
                if gross <= 0 or gross.as_tuple().exponent < -5:
                    raise ValueError('gateway accepts positive five-decimal requests')
                fee = gateway.fee(gross)
                balance = amount(record['balance'])
                if balance < gross + fee:
                    self.error(400, 'error.insufficientBalance')
                    return
                record['balance'] = decimal(balance - gross - fee)
                transactions[reference] = {'id': str(uuid.uuid4()), 'reference': reference,
                                           'fromAddress': body['address'], 'toAddress': cfg.treasury,
                                           'amount': decimal(gross), 'providerFee': decimal(fee),
                                           'netAmount': decimal(gross-fee), 'txStatus': 'pending',
                                           'deliveries': [], 'mock_test_only': True}
                gateway.save()
                self.reply(201, transactions[reference])
            elif route == '/__test__/fund':
                record = addresses[body['address']]
                record['balance'] = decimal(amount(record['balance']) + amount(body['amount']))
                gateway.save()
                self.reply(200, {'balance': record['balance'], 'mock_test_only': True})
            elif route == '/__test__/delivery':
                if not isinstance(body.get('fail'), bool):
                    raise ValueError('fail must be boolean')
                gateway.state['fail_delivery'] = body['fail']
                gateway.save()
                self.reply(200, {'fail_delivery': body['fail'], 'mock_test_only': True})
            elif route in ('/__test__/confirm', '/__test__/fail'):
                record = transactions[body['reference']]
                status = 'confirmed' if route.endswith('/confirm') else 'failed'
                if record.get('original_raw_body') and record['txStatus'] != status:
                    self.error(409, 'test.terminalTransactionCannotChange')
                    return
                if not record.get('original_raw_body'):
                    event = {key: record[key] for key in ('reference', 'fromAddress', 'toAddress', 'amount', 'providerFee', 'netAmount')}
                    event.update({'type': 'consolidate', 'status': status, 'operationType': 'api',
                                  'transactionCompletedAt': dt.datetime.now(dt.timezone.utc).isoformat().replace('+00:00', 'Z'),
                                  'errorMessage': None if status == 'confirmed' else 'MOCK TEST ONLY forced failed transaction',
                                  'txHash': '0x' + hashlib.sha256(('MOCK TX:' + body['reference']).encode()).hexdigest(),
                                  'tokenName': cfg.token, 'tokenAddress': cfg.token_address, 'blockchain': cfg.network})
                    record['txStatus'] = status
                    record['original_raw_body'] = json.dumps(event, separators=(',', ':'))
                    record['original_signature'] = hmac.new(cfg.webhook_secret.encode(), record['original_raw_body'].encode(), hashlib.sha256).hexdigest()
                    gateway.save()
                result = {'reference': body['reference'], 'txStatus': status, 'mock_test_only': True}
                if body.get('deliver', True):
                    result['delivery'] = gateway.deliver(body['reference'])
                self.reply(200, result)
            else:
                self.error(404, 'test.routeNotFound')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--host', default='127.0.0.1')
    parser.add_argument('--port', type=int, default=18781)
    parser.add_argument('--private-docker', action='store_true')
    parser.add_argument('--callback', default='http://127.0.0.1:18082/v1/webhooks/cryptorails')
    parser.add_argument('--state-dir', required=True)
    parser.add_argument('--network', default='local-test-only')
    parser.add_argument('--token', default='USDC', choices=['USDC', 'USDT'])
    parser.add_argument('--vm-type', default='EVM', choices=['EVM'])
    parser.add_argument('--decimals', type=int, default=6)
    parser.add_argument('--token-address', default='0x'+'1'*40)
    parser.add_argument('--treasury', default='0x'+'2'*40)
    parser.add_argument('--fixed-fee', default='0.10')
    parser.add_argument('--fee-bps', type=int, default=100)
    cfg = parser.parse_args()
    if cfg.host not in ('127.0.0.1', '::1') and not (cfg.private_docker and cfg.host == '0.0.0.0'):
        parser.error('bind loopback, or explicitly use --host 0.0.0.0 --private-docker inside a private container')
    callback = urllib.parse.urlsplit(cfg.callback)
    allowed = False
    try:
        address = ipaddress.ip_address(callback.hostname or '')
        allowed = address.is_loopback or cfg.private_docker and address.is_private
    except ValueError:
        allowed = cfg.private_docker and bool(callback.hostname) and '.' not in callback.hostname
    if callback.scheme != 'http' or not allowed or callback.username or callback.password or callback.fragment:
        parser.error('test callback must be local HTTP/private Docker; real public callback URLs are forbidden')
    if not 5 <= cfg.decimals <= 18 or not 0 <= cfg.fee_bps <= 9999 or amount(cfg.fixed_fee) < 0:
        parser.error('invalid test token/fee configuration')
    cfg.api_key = os.environ.get('MOCK_CRYPTORAILS_API_KEY', 'mock-tenant-key')
    cfg.webhook_secret = os.environ.get('MOCK_CRYPTORAILS_WEBHOOK_SECRET', 'mock-webhook-secret')
    server = ThreadingHTTPServer((cfg.host, cfg.port), Handler)
    server.gateway = Gateway(cfg)
    print(f'MOCK TEST ONLY gateway http://{cfg.host}:{cfg.port}/v1; chain={cfg.network}; no blockchain or real money', flush=True)
    server.serve_forever()


if __name__ == '__main__':
    main()
