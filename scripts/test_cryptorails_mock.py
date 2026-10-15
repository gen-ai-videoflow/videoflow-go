#!/usr/bin/env python3
"""Offline mock contract tests: no real gateway, wallet or blockchain."""
import argparse
import hashlib
import hmac
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import tempfile
import threading
import unittest
import urllib.error
import urllib.request

from cryptorails_mock import Gateway, Handler


class Callback(BaseHTTPRequestHandler):
    def do_POST(self):
        self.server.received.append((self.rfile.read(int(self.headers['Content-Length'])), self.headers['x-cr-webhook-signature']))
        self.send_response(200)
        self.end_headers()
    def log_message(self, *args):
        pass


class MockContractTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.callback = ThreadingHTTPServer(('127.0.0.1', 0), Callback)
        self.callback.received = []
        self.cfg = argparse.Namespace(state_dir=self.directory.name, api_key='mock-tenant-key', webhook_secret='mock-webhook-secret',
            network='local-test-only', vm_type='EVM', token='USDC', decimals=6, token_address='0x'+'1'*40,
            treasury='0x'+'2'*40, fee_bps=100, fixed_fee='0.10', callback=f'http://127.0.0.1:{self.callback.server_port}/callback')
        self.gateway = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        self.gateway.gateway = Gateway(self.cfg)
        for server in (self.callback, self.gateway):
            threading.Thread(target=server.serve_forever, daemon=True).start()
        self.base = f'http://127.0.0.1:{self.gateway.server_port}'

    def tearDown(self):
        for server in (self.gateway, self.callback):
            server.shutdown()
            server.server_close()
        self.directory.cleanup()

    def request(self, path, body=None, key='mock-tenant-key'):
        request = urllib.request.Request(self.base+path, data=None if body is None else json.dumps(body).encode(),
            headers={'x-tenant-api-key': key, 'Content-Type': 'application/json'})
        try:
            with urllib.request.urlopen(request, timeout=5) as response:
                return response.status, json.loads(response.read())
        except urllib.error.HTTPError as error:
            result = error.code, json.loads(error.read())
            error.close()
            return result

    def test_policy_address_and_authentication(self):
        self.assertEqual(self.request('/__test__/state', key='')[0], 401)
        self.assertEqual(self.request('/__test__/delivery', {'fail': True}, key='')[0], 401)
        status, policy = self.request('/v1/api/tenant-token-statuses?blockchainName=local-test-only&tokenAddress='+self.cfg.token_address)
        self.assertEqual(status, 200)
        self.assertEqual(policy['data'][0]['tokenConfig']['decimal'], 6)
        self.assertEqual(self.request('/v1/api/tenant-token-statuses?blockchainName=wrong&tokenAddress='+self.cfg.token_address)[1]['data'], [])
        status, address = self.request('/v1/api/create-address', {'vmType': 'EVM', 'reference': 'owned-reference'})
        self.assertEqual(status, 201)
        self.assertEqual(address['vmType']['type'], 'EVM')
        self.assertTrue(address['vmType']['isActive'])
        self.assertEqual(self.request('/v1/api/create-address', {'vmType': 'EVM', 'reference': 'owned-reference'})[0], 409)
        self.assertEqual(self.request('/v1/api/address?address='+address['address']+'&reference=wrong')[0], 404)

    def test_balance_stable_reference_failed_delivery_original_replay(self):
        address = self.request('/v1/api/create-address', {'vmType': 'EVM', 'reference': 'invoice-1'})[1]['address']
        command = {'address': address, 'amount': '10', 'transactionReference': 'invoice-1',
                   'blockchainName': self.cfg.network, 'tokenAddress': self.cfg.token_address}
        status, response = self.request('/v1/api/consolidate', command)
        self.assertEqual((status, response['message']), (400, 'error.insufficientBalance'))
        self.request('/__test__/fund', {'address': address, 'amount': '10.20'})
        self.assertEqual(self.request('/v1/api/consolidate', command)[0], 201)
        self.assertEqual(self.request('/v1/api/consolidate', command)[0], 409)
        self.request('/__test__/delivery', {'fail': True})
        result = self.request('/__test__/confirm', {'reference': 'invoice-1'})[1]
        self.assertEqual(result['delivery']['status'], 'failed')
        self.assertEqual(len(self.callback.received), 0)
        original = self.request('/__test__/events/invoice-1')[1]
        self.assertEqual(json.loads(original['raw_body'])['netAmount'], '9.800000000000000000')
        self.assertEqual(self.request('/v1/api/transaction?reference=invoice-1')[1]['txStatus'], 'confirmed')
        # Explicit replay; no timer/background automatic retry exists.
        self.request('/__test__/delivery', {'fail': False})
        for _ in range(2):
            self.assertEqual(self.request('/__test__/deliver', {'reference': 'invoice-1'})[1]['http_status'], 200)
        expected = original['raw_body'].encode()
        self.assertEqual(self.callback.received, [(expected, original['signature'])]*2)
        self.assertEqual(original['signature'], hmac.new(self.cfg.webhook_secret.encode(), expected, hashlib.sha256).hexdigest())
        stored = json.loads((Path(self.directory.name)/'gateway-state.json').read_text())
        self.assertEqual(stored['transactions']['invoice-1']['original_raw_body'], original['raw_body'])

    def test_failed_transaction_is_terminal(self):
        address = self.request('/v1/api/create-address', {'vmType': 'EVM', 'reference': 'failed-invoice'})[1]['address']
        self.request('/__test__/fund', {'address': address, 'amount': '11'})
        self.request('/v1/api/consolidate', {'address': address, 'amount': '10', 'transactionReference': 'failed-invoice',
            'blockchainName': self.cfg.network, 'tokenAddress': self.cfg.token_address})
        self.assertEqual(self.request('/__test__/fail', {'reference': 'failed-invoice', 'deliver': False})[0], 200)
        self.assertEqual(self.request('/v1/api/transaction?reference=failed-invoice')[1]['txStatus'], 'failed')
        self.assertEqual(self.request('/__test__/confirm', {'reference': 'failed-invoice'})[0], 409)


if __name__ == '__main__':
    unittest.main()
