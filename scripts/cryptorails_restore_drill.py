#!/usr/bin/env python3
"""LOCAL MOCK ONLY backup/restore and signed replay drill.

Reads the disposable videoflow_crypto_qa DB only. Creates a fresh restore DB;
never restores over or drops the source, videoflow_app, or the normal test DB.
Requires Docker and the QA mock/API containers already running; no Python deps.
"""
import argparse
import datetime as dt
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


def run(arguments, **kwargs):
    return subprocess.run(arguments, check=True, capture_output=True, **kwargs)


def request(url, key=None, raw=None, signature=None):
    headers = {'Content-Type': 'application/json'}
    if key:
        headers['x-tenant-api-key'] = key
    if signature:
        headers['x-cr-webhook-signature'] = signature
    with urllib.request.urlopen(urllib.request.Request(url, data=raw, headers=headers), timeout=20) as response:
        return response.status, json.loads(response.read())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-api', default='videoflow-crypto-qa-api')
    parser.add_argument('--postgres-container', default='videoflow-saas-test-db')
    parser.add_argument('--pg-user', default='videoflow')
    parser.add_argument('--network', default='videoflow-saas-local')
    parser.add_argument('--mock-url', default='http://127.0.0.1:18781')
    parser.add_argument('--output-dir', required=True)
    parser.add_argument('--checkpoint-dump', '--snapshot', dest='checkpoint_dump', help='optional earlier QA-only custom-format dump')
    parser.add_argument('--cleanup-target', action='store_true', help='drop only this newly created restore DB after success')
    args = parser.parse_args()
    if args.source_api != 'videoflow-crypto-qa-api' or not re.fullmatch(r'videoflow[-_][a-z0-9_-]+', args.postgres_container):
        parser.error('only the explicitly named disposable QA source is supported')
    mock = urllib.parse.urlsplit(args.mock_url)
    if mock.scheme != 'http' or mock.hostname not in ('127.0.0.1', 'localhost', '::1') or mock.username:
        parser.error('mock URL must be local HTTP without credentials')
    info = json.loads(run(['docker', 'inspect', args.source_api]).stdout)[0]
    original = dict(item.split('=', 1) for item in info['Config']['Env'])
    source = urllib.parse.urlsplit(original.get('DATABASE_URL', ''))
    source_db = source.path.lstrip('/')
    if source_db != 'videoflow_crypto_qa' or original.get('CLOUD_DEV_MODE') != 'true' or original.get('CRYPTORAILS_NETWORK') != 'local-test-only':
        parser.error('source must be the disposable mock-only development QA database')
    if original.get('CRYPTORAILS_API_KEY') != 'mock-tenant-key' or original.get('CRYPTORAILS_WEBHOOK_SECRET') != 'mock-webhook-secret':
        parser.error('real tenant credentials are forbidden in this drill')
    if source.hostname != args.postgres_container:
        parser.error('source DSN host must match the explicitly selected PostgreSQL container')
    directory = Path(args.output_dir)
    directory.mkdir(mode=0o700)
    prefix = ['docker', 'exec', args.postgres_container]
    def sql(database, statement):
        result = run(prefix+['psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', args.pg_user, '-d', database, '-c', statement])
        return json.loads(result.stdout)
    def snapshot(database):
        return sql(database, """SELECT json_build_object(
 'receipts',(SELECT coalesce(json_agg(s ORDER BY reference),'[]'::json) FROM (SELECT reference,payload_hash,tx_hash FROM cloud_crypto_settlements)s),
 'accounts',(SELECT coalesce(json_agg(a ORDER BY workspace_id),'[]'::json) FROM (SELECT workspace_id,balance,reserved,debt FROM cloud_credit_accounts)a),
 'premium',(SELECT coalesce(json_agg(p ORDER BY user_id),'[]'::json) FROM (SELECT user_id,paid_through::text FROM cloud_crypto_premium)p))""")
    invoices = sql(source_db, "SELECT coalesce(json_agg(i ORDER BY id),'[]'::json) FROM (SELECT id,reference,purpose,credits,user_id,workspace_id,status FROM cloud_crypto_invoices)i")
    events = []
    for invoice in invoices:
        try:
            _, saved = request(args.mock_url+'/__test__/events/'+urllib.parse.quote(invoice['reference'], safe=''), key='mock-tenant-key')
        except urllib.error.HTTPError as error:
            error.close()
            if error.code == 404:
                continue
            raise
        payload = json.loads(saved['raw_body'])
        if payload.get('status') == 'confirmed' and payload.get('reference') == invoice['reference']:
            events.append((invoice, saved))
    if not events:
        raise RuntimeError('no confirmed mock invoices found; complete a local QA purchase first')
    backup = directory/'qa-checkpoint.dump'
    if args.checkpoint_dump:
        # Caller must supply a dump from the same QA-only environment. The source
        # and target remain explicitly guarded regardless of dump contents.
        source_dump = Path(args.checkpoint_dump)
        backup.write_bytes(source_dump.read_bytes())
    else:
        backup.write_bytes(run(prefix+['pg_dump', '-U', args.pg_user, '-d', source_db, '--format=custom', '--no-owner', '--no-acl']).stdout)
    backup.chmod(0o600)
    target = 'videoflow_crypto_restore_'+uuid.uuid4().hex[:12]
    container = 'videoflow-crypto-restore-'+uuid.uuid4().hex[:10]
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    run(prefix+['createdb', '-U', args.pg_user, '--template=template0', target])
    run(['docker', 'exec', '-i', args.postgres_container, 'pg_restore', '-U', args.pg_user, '-d', target,
         '--no-owner', '--no-acl', '--exit-on-error', '--single-transaction'], input=backup.read_bytes())
    initial = snapshot(target)
    restored_invoices = sql(target, "SELECT coalesce(json_agg(reference),'[]'::json) FROM cloud_crypto_invoices")
    events = [(invoice, saved) for invoice, saved in events if invoice['reference'] in restored_invoices]
    if not events:
        raise RuntimeError('checkpoint has no invoice matching a preserved confirmed mock callback')
    # Copy only explicit mock configuration, never arbitrary live environment.
    values = {key: value for key, value in original.items() if key.startswith('CRYPTORAILS_')}
    values.update({'DATABASE_URL': urllib.parse.urlunsplit(source._replace(path='/'+target)), 'REDIS_URL': '',
        'CLOUD_DEV_MODE': 'true', 'PUBLIC_ORIGIN': f'http://127.0.0.1:{port}', 'PORT': '8080',
        'BILLING_ENABLED': 'true', 'PREMIUM_PRICE_CENTS': original.get('PREMIUM_PRICE_CENTS', '900'),
        'PROVIDER_DAILY_BUDGET_MICROS': '0', 'PROVIDER_MAX_COST_MICROS': '0',
        'OPENROUTER_API_KEY': '', 'STRIPE_SECRET_KEY': '', 'STRIPE_WEBHOOK_SECRET': '',
        'CASH_PAYOUTS_ENABLED': 'false', 'REWARDS_ENABLED': 'false', 'CONNECT_ENABLED': 'false',
        'S3_BUCKET_ORIGINALS': '', 'MAIL_FROM': ''})
    environment = directory/'restore-api.env'
    environment.write_text(''.join(f'{key}={value}\n' for key, value in values.items()))
    environment.chmod(0o600)
    command = ['docker', 'run', '--detach', '--name', container, '--network', args.network,
               '-p', f'127.0.0.1:{port}:8080', '--env-file', str(environment), info['Image']]
    running = False
    try:
        run(command)
        running = True
        base = f'http://127.0.0.1:{port}'
        for _ in range(40):
            try:
                if request(base+'/health')[0] == 200:
                    break
            except (urllib.error.URLError, TimeoutError, ConnectionError):
                time.sleep(0.5)
        else:
            raise RuntimeError('temporary restored API failed health check; inspect its container logs')
        statuses = []
        for invoice, saved in events:
            status, _ = request(base+'/v1/webhooks/cryptorails', raw=saved['raw_body'].encode(), signature=saved['signature'])
            statuses.append({'reference': invoice['reference'], 'status': status})
        first = snapshot(target)
        for _, saved in events:
            request(base+'/v1/webhooks/cryptorails', raw=saved['raw_body'].encode(), signature=saved['signature'])
        second = snapshot(target)
        if first != second:
            raise RuntimeError('replay changed restored receipts, credit balances or Premium paid-through')
        before_refs = {value['reference'] for value in initial['receipts']}
        before_balances = {value['workspace_id']: value['balance'] for value in initial['accounts']}
        after_balances = {value['workspace_id']: value['balance'] for value in first['accounts']}
        expected_deltas = {}
        for invoice, _ in events:
            if invoice['purpose'] == 'credits' and invoice['reference'] not in before_refs:
                workspace = invoice['workspace_id']
                expected_deltas[workspace] = expected_deltas.get(workspace, 0) + invoice['credits']
        for workspace, delta in expected_deltas.items():
            if after_balances.get(workspace, 0) - before_balances.get(workspace, 0) != delta:
                raise RuntimeError('restored credit balance did not increase by the exact purchased credits')
        final_refs = {value['reference'] for value in second['receipts']}
        for invoice, _ in events:
            if invoice['reference'] not in final_refs:
                raise RuntimeError('confirmed reference did not settle after restore')
        if all(invoice['reference'] in before_refs for invoice, _ in events) and initial != second:
            raise RuntimeError('settled checkpoint changed when original receipts were replayed')
        evidence = {'mock_test_only': True, 'result': 'passed', 'source_database': source_db,
            'restore_database': target, 'backup_file': str(backup), 'restore_api_image': info['Image'],
            'confirmed_events_replayed': len(events), 'post_restore_settlement_receipts': len(second['receipts']),
            'already_settled_at_checkpoint': all(invoice['reference'] in before_refs for invoice, _ in events),
            'second_replay_unchanged': first == second, 'credit_deltas_verified': expected_deltas, 'http_results': statuses,
            'created_at': dt.datetime.now(dt.timezone.utc).isoformat(),
            'verification_scope': 'Disposable local PostgreSQL + real local Go API + original mock HMAC payloads; no AWS, chain or live gateway.'}
        (directory/'evidence.json').write_text(json.dumps(evidence, indent=2)+'\n')
        print(json.dumps(evidence, indent=2))
    finally:
        if running:
            run(['docker', 'rm', '--force', container])
        environment.unlink(missing_ok=True)
    if args.cleanup_target:
        run(prefix+['dropdb', '-U', args.pg_user, target])
        print('Dropped only newly created disposable restore database '+target)
    else:
        print('Preserved disposable restore database '+target+' for inspection')


if __name__ == '__main__':
    main()
