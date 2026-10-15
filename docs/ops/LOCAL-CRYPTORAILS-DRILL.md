# Local CryptoRails recovery drill

This exercise uses a deliberately fake gateway, disposable PostgreSQL data, and the real local Go payment handlers. It does not send cryptocurrency, call a live gateway, validate a funded tenant, or verify AWS recovery. Keep production payment activation disabled until the separate execution checklist is completed.

## Reproduce the gateway

Python 3 and its standard library are sufficient:

```sh
python3 scripts/cryptorails_mock.py \
  --host 127.0.0.1 --port 18781 \
  --callback http://127.0.0.1:18082/v1/webhooks/cryptorails \
  --state-dir /tmp/videoflow-cryptorails-mock
python3 scripts/test_cryptorails_mock.py
```

For an isolated Docker network, explicitly use `--host 0.0.0.0 --private-docker --port 8080`, publish only `127.0.0.1:18781:8080`, and set the callback to the private API service URL. Never publish this test service on a public interface. All gateway and test-control endpoints require `x-tenant-api-key: mock-tenant-key`. The default signing secret is `mock-webhook-secret`; both are mock-only credentials. Optional environment overrides are `MOCK_CRYPTORAILS_API_KEY` and `MOCK_CRYPTORAILS_WEBHOOK_SECRET`.

The matching local API configuration is `CRYPTORAILS_NETWORK=local-test-only`, VM `EVM`, token `USDC`, decimals `6`, token address `0x1111111111111111111111111111111111111111`, treasury `0x2222222222222222222222222222222222222222`, fixed fee `0.10`, and fee BPS `100`. Point its gateway URL at the mock's private `/v1` URL. These values are test fixtures, not deployable network configuration.

Implemented gateway contracts include filtered tenant token status, nested address VM metadata, reference-owned address lookup, insufficient-balance rejection, stable consolidation references, and transaction inspection. The mock retains the original webhook bytes and HMAC in its private state directory. Terminal transaction status cannot be changed, and failed delivery never schedules retries.

Authenticated test controls are:

| Endpoint | JSON body / result |
| --- | --- |
| `POST /__test__/fund` | `{ "address": "0x…", "amount": "10.20" }` adds mock balance only |
| `POST /__test__/delivery` | `{ "fail": true }` withholds callback delivery; set false before manual replay |
| `POST /__test__/confirm` | `{ "reference": "invoice-id", "deliver": true }` records confirmed original signed callback |
| `POST /__test__/fail` | Same shape, records a failed terminal transaction |
| `POST /__test__/deliver` | `{ "reference": "invoice-id" }` manually resends original bytes and signature |
| `GET /__test__/events/{reference}` | Original raw body, signature, and delivery results |
| `GET /__test__/state` | Mock fixture state |

Generate invoices through the real application's payment UI/API. Fund only their returned mock addresses, then notify the application, confirm the resulting consolidation, and inspect the application balance or Premium status. Browser polling and notify calls must never grant value before an authenticated settlement callback.

## Disposable backup and replay

The restore helper requires Docker, the existing PostgreSQL container, and the local mock/API. Its default source is explicitly restricted to `videoflow-crypto-qa-api` using database `videoflow_crypto_qa`, development mode, the mock credentials above, and the `local-test-only` network. It refuses the normal application/test database. It reads the source, creates a new uniquely named restore database, and starts a temporary real API using the source image digest. It never restores over or drops the source.

```sh
python3 scripts/cryptorails_restore_drill.py \
  --output-dir /tmp/videoflow-crypto-restore-new-run

# Replay a later signed credit settlement into an earlier pending-invoice backup:
python3 scripts/cryptorails_restore_drill.py \
  --snapshot /tmp/videoflow-crypto-qa-before.dump \
  --output-dir /tmp/videoflow-crypto-restore-earlier-run
```

Output directories must be new. Supply only a custom-format dump from this disposable QA environment to `--snapshot` (also named `--checkpoint-dump`). The helper filters callbacks to invoices present in that checkpoint, verifies exact newly purchased credit deltas, and sends every preserved original callback twice. It compares settlement receipts, credit balances/reservations/debt, and Premium paid-through values to ensure the second replay changes nothing. A previously settled checkpoint must remain unchanged after both replays. Temporary API containers and their environment files are removed; the new database is retained for inspection unless `--cleanup-target` is supplied. Dumps contain disposable test account data: retain them privately and remove them after review.

## Observed results, 15 October 2026

The mock's three automated tests passed, covering authentication and ownership, filtered policy, insufficient funds, stable consolidation references, withheld delivery, original HMAC replay, persisted originals, and terminal failed transactions. Python compilation passed for all three tooling scripts.

The parent QA session created the credit and Premium invoices through the real browser UI against the real local API. This document's restore runs independently verified:

| Checkpoint | Result | Local evidence |
| --- | --- | --- |
| Both invoices settled | Two original signed callbacks replayed twice; two receipts retained; balances and Premium paid-through unchanged | `/tmp/videoflow-crypto-restore-settled-20261015/evidence.json` |
| Credit invoice pending before payment | Original signed callback added exactly 1000 credits once; second replay unchanged; one receipt | `/tmp/videoflow-crypto-restore-pending-20261015/evidence.json` |

The restored databases are `videoflow_crypto_restore_c1734e44f16d` and `videoflow_crypto_restore_283f0ce98827`. Both runs used image `sha256:9bad2d082eb2fd016c28bb4091393f968c2bfcd2a5d46541e361528ac8ee7d96`. Evidence files are local artifacts, not committed credentials or proof of production readiness. No live gateway, blockchain, AWS restore, or production payment activation was exercised.
