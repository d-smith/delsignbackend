# Delsignbackend - Delegated Signing Backend

Simple backend for delegated signing example. 


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

```bash
export DELSIGN_JWT_SECRET=$(openssl rand -hex 32)
export DELSIGN_KEY_ENC_KEY=$(openssl rand -hex 32)
go run .
```

Every endpoint needs an `Authorization: Bearer <token>` header. The token must be an
HS256 JWT signed with `DELSIGN_JWT_SECRET`, with a non-empty `email` claim and an `exp`
claim; tokens without `exp`, expired tokens, and other algorithms get a 403.

The server stops cleanly on Ctrl-C or SIGTERM: it lets in-flight requests finish (up to
10 seconds), then closes the database.


## Signing protocol

A valid JWT alone can't move funds: key changes and sends also need a signature from the
user's RSA key. All signatures are RSA PKCS #1 v1.5 over the SHA-256 of a UTF-8 message,
hex encoded. Public keys are hex-encoded PKIX DER.

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

**Look up your registration** (`GET /api/v1/users/{email}`): only your own email; any other
gets a 403.


## Dev chain

Local development uses [Anvil](https://getfoundry.sh/anvil/overview) from Foundry. Install
Foundry with `curl -L https://foundry.paradigm.xyz | bash`, then `foundryup`. The prebuilt
binaries need glibc 2.32 or newer (Ubuntu 22.04+).

```bash
anvil
```

Anvil serves JSON-RPC on `http://localhost:8545` with chain ID 31337, which match the
defaults above. It starts with ten unlocked, pre-funded dev accounts. To fund an address
the server created, send from one of them:

```bash
cast send --unlocked --from 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266 <address> --value 1ether
```

Anvil's state is in memory, so restarting it resets all balances.


## Smoke test

`scripts/smoke.sh` runs the main flow end to end against a running dev node: it builds and
starts the server on a scratch database, registers an RSA key, creates a wallet with two
addresses, funds one from the node's first account, sends ETH to the other through the API,
and checks the balance. It needs `go`, `curl`, `jq` and `openssl`.

```bash
anvil &
scripts/smoke.sh
```

`SMOKE_RPC_URL` (default `http://localhost:8545`) and `SMOKE_PORT` (default `3099`) override
the node and server port; `SMOKE_KEEP=1` keeps the temp directory with the DB and server log.


## Sqlite3 DB  

For simple PoC work this version uses a sqlite3 database.

The server creates the database file (`DELSIGN_DB_PATH`) and its tables on startup if they
don't exist, using `db/schema.sql`, which is embedded in the binary. Existing tables and data
are left as they are.

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

To interact with the database:

```
sqlite3 delsign.db
```

In the absence of an address book, this query lists other wallet addresses for the current user:

``` 
select address from addresses where wallet_id in (select id from wallets where email='doug.smith.mail@gmail.com')

0xB7Ed5c8B13176150b9b7A6fA3027Bc4818236Ed5
0xbD9604320bfcDabB5F6FBA760d01BbaDb989d229
0x3EDE0E50a0979DAc615Ee9c7C38545d15cd582a0
0xD3Cf3Bfa79b09E091631C5F6930fB14F7Cb68db5
0x0289E2030DBFEE0FDD4AC1A5b1Dd474B13Ac599E
0x73dA1eD554De26C467d97ADE090af6d52851745E
```