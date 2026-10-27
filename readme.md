# Delsignbackend - Delegated Signing Backend

Simple backend for delegated signing example. 


## Configuration

The server reads its settings from environment variables at startup:

| Variable | Default | Notes |
|---|---|---|
| `DELSIGN_JWT_SECRET` | none, **required** | HMAC secret for validating JWTs. Must be at least 32 bytes and match the secret your token issuer uses. |
| `DELSIGN_PORT` | `3010` | HTTP listen port |
| `DELSIGN_DB_PATH` | `delsign.db` | SQLite database file |
| `DELSIGN_RPC_URL` | `http://localhost:8545` | Ethereum JSON-RPC endpoint |
| `DELSIGN_CHAIN_ID` | `31337` | Chain ID used when signing transactions |

```bash
export DELSIGN_JWT_SECRET=$(openssl rand -hex 32)
go run .
```


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