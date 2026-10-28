# Delsignbackend - Delegated Signing Backend

A proof-of-concept backend for **delegated signing**: the server holds Ethereum private keys
for its users and signs transactions for them, but only when the user approves each one with
their own RSA key.

1. An identity provider issues the user an HS256 JWT with an `email` claim.
2. The user registers an RSA public key, proving they hold it by signing their email.
3. The user creates wallets and addresses; the server generates a secp256k1 key for each
   address and stores it encrypted.
4. To send ETH, the user signs the request with their RSA key. The server checks the
   signature, that the user owns the source address, and that the request is fresh, then
   signs and broadcasts an EIP-1559 transaction.

A valid JWT alone is never enough to move funds or replace the user's RSA key.

This is a PoC (SQLite, a local dev chain, one process), not a production custody system. See
[plan/roadmap.md](plan/roadmap.md) for known limitations.


## Quick start

Needs Go 1.27.3 (`go.mod` selects it automatically with the default `GOTOOLCHAIN=auto`) and,
for a dev chain, [Anvil](#dev-chain).

```bash
anvil &
export DELSIGN_JWT_SECRET=$(openssl rand -hex 32)
export DELSIGN_KEY_ENC_KEY=$(openssl rand -hex 32)
go run .
```


## Configuration

The server reads its settings from environment variables at startup:

| Variable | Default | Notes |
|---|---|---|
| `DELSIGN_JWT_SECRET` | none, **required** | HMAC secret for validating JWTs. Must be at least 32 bytes and match the secret your token issuer uses. |
| `DELSIGN_KEY_ENC_KEY` | none, **required** | AES-256 key that encrypts the stored Ethereum private keys: 64 hex characters. Losing it makes every stored key unusable. |
| `DELSIGN_PORT` | `3010` | HTTP listen port |
| `DELSIGN_DB_PATH` | `delsign.db` | SQLite database file |
| `DELSIGN_RPC_URL` | `http://localhost:8545` | Ethereum JSON-RPC endpoint |
| `DELSIGN_CHAIN_ID` | `31337` | Chain ID used when signing transactions |

The server stops cleanly on Ctrl-C or SIGTERM: it lets in-flight requests finish (up to
10 seconds), then closes the database.


## API

Every endpoint needs an `Authorization: Bearer <token>` header. The token must be an
HS256 JWT signed with `DELSIGN_JWT_SECRET`, with a non-empty `email` claim and an `exp`
claim; tokens without `exp`, expired tokens, and other algorithms get a 403.

| Endpoint | Does | Success |
|---|---|---|
| `POST /api/v1/keyreg` | Register or replace your RSA key ([below](#signing-protocol)) | 200 |
| `GET /api/v1/users/{email}` | Your own registration, `{"email", "pubkey"}`; any other email → 403 | 200 |
| `POST /api/v1/wallets` | Create a wallet, `{"id": 1}` | 201 |
| `GET /api/v1/wallets` | Your wallet ids, `[1, 2]` | 200 |
| `POST /api/v1/wallets/{id}/addresses` | New address in your wallet, `{"eoa": "0x…"}` | 201 |
| `GET /api/v1/walletctx` | All your wallets and addresses, `[{"walletId", "address"}]` | 200 |
| `GET /api/v1/wallets/balance/{address}` | Balance in wei, `{"address", "amount"}` | 200 |
| `POST /api/v1/wallets/send` | Send ETH ([below](#signing-protocol)), `{"txnid": "0x…"}` | 200 |
| `GET /api/v1/authping` | Check a token | 200 |

Errors are plain text. 400 means a malformed request, 403 a missing or bad token or a
resource that isn't yours, and 502 an error from the Ethereum node. Request bodies must be
JSON with exactly the documented fields; unknown fields get a 400.


## Signing protocol

Key changes and sends need a signature from the user's RSA key as well as the JWT. All
signatures are RSA PKCS #1 v1.5 over the SHA-256 of a UTF-8 message, hex encoded. Public keys
are hex-encoded PKIX DER.

**Register a key** (`POST /api/v1/keyreg`):

```json
{"email": "a@example.com", "pubkey": "<hex>", "sig4reg": "<new key's signature over email>"}
```

The first registration needs only the JWT. **Replacing** a registered key also needs
`"sig4rotate"`: the *current* key's signature over `email + pubkey` (the new key's hex,
appended). Without it, or with a bad one, the request gets a 403. A user who loses their
key can't replace it; there is no recovery path yet.

**Send ETH** (`POST /api/v1/wallets/send`):

```json
{"source": "0x…", "dest": "0x…", "amount": 1000, "requestId": "<UUID>", "expiry": 1800000060, "sig": "<hex>"}
```

`sig` is over `source + dest + amount + requestId + expiry` concatenated, with `amount` and
`expiry` in decimal, e.g. `0xAb…0xCd…10006f1c2a7e-0b9d-4c55-9a43-2f1e8d7c6b5a1800000060`.
`requestId` is a fresh UUID for each request; `expiry` is a Unix time in seconds, in the future
and at most 5 minutes ahead (a minute or two leaves room for clock skew). Expired requests
or a bad `requestId` get a 400, and a `requestId` that was already used gets a 409. (This is
not the Ethereum transaction nonce, which the server gets from the node.)

`amount` is a positive integer in wei, as a JSON number. `source` and `dest` may be in any
letter case, but `sig` covers them exactly as written in the request. Responses:

| Status | When |
|---|---|
| 200 | Broadcast; the body is `{"txnid": "0x…"}` |
| 400 | Malformed body, bad address, non-positive amount, bad or expired `requestId`/`expiry` |
| 401 | Signature doesn't verify |
| 403 | `source` isn't one of your addresses |
| 409 | `requestId` already used |
| 502 | The Ethereum node rejected the transaction or couldn't be reached; the body has its error |

The server sends an EIP-1559 transaction: the tip is the node's suggestion, and the fee cap is
twice the latest base fee plus the tip. Concurrent sends from one address are serialized,
so each one gets its own account nonce.


## Dev chain

Local development uses [Anvil](https://getfoundry.sh/anvil/overview) from Foundry. Install
Foundry with `curl -L https://foundry.paradigm.xyz | bash`, then `foundryup`. The prebuilt
binaries need glibc 2.32 or newer (Ubuntu 22.04+).

```bash
anvil
```

Anvil serves JSON-RPC on `http://localhost:8545` with chain ID 31337, which matches the
defaults above. It starts with ten unlocked, pre-funded dev accounts. To fund an address
the server created, send from one of them:

```bash
cast send --unlocked --from 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266 <address> --value 1ether
```

Anvil's state is in memory, so restarting it resets all balances.


## Database

The server uses SQLite. It creates the database file (`DELSIGN_DB_PATH`) and its tables on
startup if they don't exist, using `db/schema.sql`, which is embedded in the binary. Existing
tables and data are left as they are; there are no migrations, so a schema change to an
existing table needs a new database file.

Private keys are stored encrypted with `DELSIGN_KEY_ENC_KEY` (`enc:v1:…` in the
`private_key` column). Rows from before encryption are not readable; start from a new
database file.

To add an existing key to a wallet, for example an Anvil dev account, use `import-key`.
It reads the hex key from stdin, encrypts it, and prints the address:

```bash
go run . import-key -wallet 1 < account.key
```

Reading from stdin rather than a flag keeps the key out of `ps` output; keep it in a file
rather than typing it on the command line, so it stays out of shell history too. The command
uses the same environment variables as the server, and the wallet must already exist.

To look around: `sqlite3 delsign.db`. There is no address book; `GET /api/v1/walletctx` lists
all of a user's addresses.


## Development

```bash
go build ./... && go vet ./... && go test ./...
```

Tests use the standard library only: `httptest`, a temp-file SQLite database per test, and a
fake Ethereum node (`chain.Client`) for the send path. Shared helpers (test DB, JWTs, RSA
keys) are in `testutil/`.

**Smoke test.** `scripts/smoke.sh` runs the whole flow against a running dev node. It builds
and starts the server on a scratch database, then:
- registers an RSA key and looks up its own record (another user's → 403);
- creates a wallet and two addresses, and funds one from the node's first account;
- sends ETH through the API and checks the balance;
- replays the send (→ 409), and fires two concurrent sends from one address;
- imports Anvil dev account 9 with `import-key` and sends from it (skipped on nodes without
  Anvil's default accounts).

It needs `go`, `curl`, `jq` and `openssl`.

```bash
anvil &
scripts/smoke.sh
```

`SMOKE_RPC_URL` (default `http://localhost:8545`) and `SMOKE_PORT` (default `3099`) override
the node and server port; `SMOKE_KEEP=1` keeps the temp directory with the DB and server log.

**Layout.** `main.go` (routes, server, `import-key`), `config/`, `db/` (schema), `middleware/`
(auth, logging, recovery), `users/` (RSA keys), `wallets/` (wallets, addresses, key
encryption), `chain/` (balance, send, replay protection), `helpers/` (JSON in and out).

**Planning.** Work is planned in `plan/`: numbered plan files, plus
[roadmap.md](plan/roadmap.md) for known limitations and deferred work.
