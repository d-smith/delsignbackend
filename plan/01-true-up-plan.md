# 01 — Dependency True-Up and Hardening Plan

- **Status:** Agreed (2026-10-23)
- **Branch:** `feature/refresh`
- **Created:** 2026-10-23

## 1. Purpose and scope

Bring `delsignbackend` up to current, supported dependencies and fix the security and
robustness problems found in the October 2026 review. The service stays a proof of
concept: SQLite, a local dev chain, and one process. The goal is a PoC that is safe to
demo and easy to extend, not a production custody system.

**In scope**
- Dependency and toolchain upgrades, including removing unmaintained modules.
- Security fixes: secrets handling, key storage, replay protection, key registration, logging.
- Robustness fixes: panics, ignored errors, input validation, HTTP status codes, server timeouts, shutdown.
- A minimal test harness, so the changes above can be checked.

**Out of scope** (recorded so we don't drift into it)
- Replacing SQLite or adding a migrations framework.
- HSM/KMS-backed key custody, multi-tenant isolation, rate limiting.
- An address book, ERC-20 support, or other new features.
- Frontend changes, except the protocol change in D4, which needs a matching client update.

## 2. Project context

Delsignbackend is a **delegated signing** backend. The server holds Ethereum private
keys for its users. A user never signs an Ethereum transaction directly. Instead:

1. An external identity provider issues an HS256 JWT with an `email` claim.
2. The user registers an RSA public key (`POST /api/v1/keyreg`) and proves they hold it
   by signing their email.
3. The user creates wallets and addresses. The server generates and stores a secp256k1
   key for each address.
4. To send ETH, the user signs `source+dest+amount` with their RSA key. The server checks
   that signature and that the user owns `source`, then signs the Ethereum transaction
   with the stored key and broadcasts it.

The security model rests on step 4: **a valid JWT alone must not be enough to move
funds.** The user's RSA signature is the second factor. Several fixes below (D4, D5)
exist to keep that property true.

## 3. Architecture (current)

_Updated after Phase 2: one standard library `ServeMux`; negroni and the second router are gone._

```
       http.Server :3010 (timeouts, graceful shutdown)
                │
        Logging → Recovery
                │
        http.ServeMux ── GET /api/v1/users/{email}         (public until T3.5)
                │
          per route: auth(handler)  (JWT → typed ctx email)
                ┌──────────────┼───────────────────┐
             users/        wallets/             chain/
          keyreg, GetUser  wallets, addresses   balance, send
                │              │                   │
                └──── SQLite delsign.db ◄──────────┤ (one shared *sql.DB)
                                                   │
                                     ethclient ──► Anvil :8545 (chain 31337)
```

Every package has a global singleton (`users.UserDatabase`, `wallets.WalletsDatabase`,
`wallets.AddressDatabase`, `chain.EthChain`), and `main.go` sets them up. Settings are
hardcoded constants: port, DB file, RPC URL, chain ID, JWT secret.

## 4. Repo layout and conventions

```
main.go                  wiring, routes, startup and shutdown
authz/jwt.go             JWT validation
middleware/authcheck.go  auth middleware
helpers/parse.go         strict JSON body decoding
users/                   key registration handlers + users table access
wallets/                 wallet/address handlers + wallets/addresses table access
chain/ethereum.go        balance, send, signature check, tx build/sign/broadcast
db/schema.sql            schema (applied by hand)
plan/                    planning docs: NN-<name>.md per plan, roadmap.md, baseline/
```

**Existing conventions we keep**
- Packages split by domain. Each package holds its handlers and table access side by side.
- Handlers are plain `func(http.ResponseWriter, *http.Request)` and are registered in `main.go`.
  After T2.3, path parameters are read with `r.PathValue(...)` instead of `mux.Vars(r)`.
- Standard library `log` for logging. `http.Error` for error responses.
- JSON field names stay as they are on the wire (`sig4reg`, `walletId`, `txnid`, …),
  because a client already depends on them. Exception: the deliberate protocol change in D4.

**Conventions we add**
- No secrets in source or logs: no private keys, JWTs, or claims.
- Every error is handled or explicitly ignored with a comment.
- The authenticated email is read through `middleware.EmailFromContext(ctx) (string, bool)`,
  never through an unchecked type assertion.
- Request bodies are decoded with `helpers.DecodeJSONBody`.
- Each task is one commit and passes `go build ./... && go vet ./... && go test ./...`.
- Claude makes no git commits. The user reviews and commits every change.

## 5. Key design decisions

All decisions were agreed on 2026-10-23 (see the decision log in §8).

| ID | Decision | Choice | Status |
|----|----------|--------|--------|
| D1 | Configuration | Environment variables, read once at startup in a new `config` package: `DELSIGN_PORT`, `DELSIGN_DB_PATH`, `DELSIGN_RPC_URL`, `DELSIGN_CHAIN_ID`, `DELSIGN_JWT_SECRET`, `DELSIGN_KEY_ENC_KEY`. Development defaults for everything except the two secrets. Startup fails if a secret is missing. | Agreed |
| D2 | Database handles | One shared `*sql.DB`, opened in `main` and passed to each store constructor. Keep the package-level vars so handler code barely changes; tests set them to an in-memory DB. Apply the schema at startup with `CREATE TABLE IF NOT EXISTS` from an embedded `schema.sql`. | Agreed |
| D3 | Private keys at rest | Encrypt the `private_key` column with AES-256-GCM, using a key from `DELSIGN_KEY_ENC_KEY`. Stored format: `enc:v1:<hex nonce‖ciphertext>`. **Plaintext keys are not supported:** reading a row without the prefix is an error. Existing dev DBs are thrown away. To replace the readme's raw SQL seed, add an `import-key` subcommand that encrypts a key (for example an Anvil dev account) and attaches it to a wallet. | Agreed |
| D4 | Replay protection on send | **Breaking change for the client, hard cutover** (no period where both formats work). The signed message becomes `source+dest+amount+nonce+expiry`. `nonce` is a client-generated UUID and `expiry` is a Unix timestamp, at most 5 minutes in the future. The server rejects expired requests and nonces it has already seen; seen nonces go in a `used_nonces` table. | Agreed |
| D5 | Key registration and rotation | A user can register their first key with a JWT alone. **Replacing** an existing key also needs a signature from the current key. Otherwise a stolen JWT could register a new key and then send funds. **Accepted limitation:** a user who loses their key is locked out, because there is no recovery path. This is recorded in `plan/roadmap.md` (T5.2). | Agreed |
| D6 | `GET /users/{email}` | Move it behind auth, and let users look up only their own record (403 for anyone else). The frontend doesn't look up other users. | Agreed |
| D7 | JWT library | `golang-jwt/jwt/v5` with `WithValidMethods(["HS256"])`. The `exp` claim is required. | Agreed |
| D8 | HTTP stack | **Standard library only.** Remove both gorilla/mux and negroni. Routes use the `http.ServeMux` method and wildcard patterns added in Go 1.22 (`"GET /api/v1/wallets/balance/{address}"`), and handlers read path values with `r.PathValue`. Middleware is plain `func(http.Handler) http.Handler`: logging, panic recovery, and auth, which wraps only the protected routes. One mux, so the two-router setup goes away. Run the server as `http.Server` with timeouts, and shut down gracefully with `signal.NotifyContext` + `Shutdown`. | Agreed |
| D9 | Ethereum transactions | Upgrade to go-ethereum v1.17.x. Build EIP-1559 `DynamicFeeTx` transactions, sign with `types.LatestSignerForChainID`, and get the fee cap and tip from the node. | Agreed |
| D10 | Local dev chain | **Move from Ganache to Anvil** (Foundry); Ganache was sunset in 2023. Anvil serves RPC on `:8545`, uses chain ID **31337** (the default for `DELSIGN_CHAIN_ID`), supports EIP-1559, and starts with pre-funded, unlocked dev accounts. The smoke script funds test addresses from them with `eth_sendTransaction` (T1.5). | Agreed |
| D11 | Testing | Use the standard `testing` package and `httptest` with in-memory SQLite. Put the Ethereum client behind a small interface so `SendEth` can be tested without a node. No mocking framework. | Agreed |
| D12 | Toolchain | Go 1.27, the current major release. Set `go 1.27` and `toolchain go1.27.3` in `go.mod`. With `GOTOOLCHAIN=auto` (the default), the `go` command picks up this toolchain even if an older Go is installed. Developer machines should still match it, so the editor and gopls use the same version as the build. | Agreed |

## 6. Execution approach

- Work on `feature/refresh`, one task at a time, in the order of §7. Each task is one commit.
- **Commits:** Claude makes the change, runs the gates, updates §7, and summarizes the diff.
  The user reviews and commits. The next task starts only after the user says so.
- Before each task, look at the code it touches again; after it, mark it done here (§7) and
  write down anything we learned.
- **Gate for every task:** `go build ./...`, `go vet ./...`, `go test ./...` all pass.
- **Additional gates:**
  - Dependency tasks: `govulncheck ./...` shows no new reachable findings, and fewer than before.
  - Chain tasks: `scripts/smoke.sh` passes against a local `anvil` (added in T1.5).
- Limitations or deferred work found along the way are noted in §8 at once, and
  T5.2 collects them into the roadmap.
- Phases are ordered so that tests exist before the riskier refactors, and the go-ethereum
  upgrade lands before the transaction-format change that depends on it.
- If a task turns out bigger than expected, split it here before continuing.

## 7. Task list

Legend: `[ ]` todo · `[~]` in progress · `[x]` done

### Phase 0 — Baseline

- [x] **T0.1 Record baseline.** Save the current `govulncheck` summary and `go list -m -u all`
  output to `plan/baseline/`. *Accept:* files present and ready for the user to commit.
  *Done:* baseline at `e715750`, Go 1.26.5, govulncheck v1.9.0 (pinned for T5.3).
  Vulnerabilities: 24 called / 7 imported / 27 required. 136 of 166 modules have updates.
  See `plan/baseline/README.md`.
- [x] **T0.2 Toolchain + tidy.** Apply D12: set `go 1.27` and `toolchain go1.27.3`, then
  run `go mod tidy`, so that direct dependencies are no longer marked `// indirect`.
  *Prerequisite:* the user installs Go 1.27.3 on their system (`/usr/local/go`).
  *Accept:* `go version` in the repo reports go1.27.3; build and vet pass; govulncheck
  v1.9.0 shows no standard library vulnerabilities that our code reaches (the baseline
  had 16). Any new ones on 1.27.3 are listed here with a reason.
  *Done:* go-ethereum, golang-jwt/jwt, gddo and go-sqlite3 moved to the direct `require` block.
  No module versions changed (checked against the baseline). `go.sum` gained 119 lines that
  had been missing. Vulnerabilities: 24 → **8 called** (0 standard library, 0 new on 1.27.3),
  7 → 2 imported, 27 → 26 required. The 8 left are go-ethereum ×5 and gnark-crypto (T2.4),
  golang-jwt v3 (T2.1), and gorilla/websocket (T2.4/T2.5).

### Phase 1 — Foundations for change

- [x] **T1.1 Config package (D1).** Add `config/` and replace the hardcoded constants in
  `main.go`, `users/userstore.go`, `wallets/*.go`, `chain/ethereum.go` and the middleware.
  *Accept:* startup fails if `DELSIGN_JWT_SECRET` is missing; no secret literals remain in the source.
  *Done:* `config.Load()` validates the port, chain ID and secret. The secret must be at least
  32 bytes (RFC 7518 §3.2). The middleware is now built by `NewAuthzMiddleWare(secret)`, the store
  constructors take a DB path, and `EthereumChain` holds the chain ID. The chain ID default stays
  1337 until T1.5; `DELSIGN_KEY_ENC_KEY` is deferred to T3.2. Unit tests in `config/`. Verified:
  missing or short secret → exit 1; token signed with the configured secret → 200, with the old
  `"secret"` → 403. Added a Configuration section to the readme.
- [x] **T1.2 Shared DB + embedded schema (D2).** One `*sql.DB`, store constructors that take
  it, and the schema applied at startup. Also fix the `private_key test` typo in the schema.
  *Accept:* the server starts against an empty file and creates the tables.
  *Done:* new `db` package, where `db.Open(path)` opens the connection and applies the
  embedded `schema.sql`. Store constructors take the `*sql.DB`, and the per-store `Close`/`ShutdownDB`
  methods are gone (`main` closes the shared handle). Tests cover table creation and
  reopening without losing data. Verified against an empty file (tables created, wallet
  create/list work) and a copy of an existing dev DB (data kept). Readme no longer says to
  run `schema.sql` by hand. The typo fix only affects new DBs, because `CREATE TABLE IF NOT EXISTS`
  doesn't alter existing tables; that's fine, since D3 discards old DBs anyway.
- [x] **T1.3 Typed context key.** Add `middleware.EmailFromContext`, and use it everywhere
  `ctx.Value("email").(string)` appears now. *Accept:* a JWT with no `email` claim gets a 403, not a panic.
  *Done:* the middleware now requires a non-empty string `email` claim (403 otherwise) and
  stores it under an unexported typed key. All six lookups (5 handlers + `validateKeyReg`) use
  `EmailFromContext`. Before: a token with no `email` claim → panic → 500. After: 403 on every
  protected endpoint, with no panics. `middleware/authcheck_test.go` covers 7 rejection cases,
  the valid token path, and a check that a string-keyed context value can't stand in for the
  typed key.
- [x] **T1.4 Test harness (D11).** Helpers for an in-memory DB, test JWTs and test RSA keys.
  Add first tests for the success and failure paths of `validateKeyReg`. The middleware tests
  were added in T1.3; move their token helper into the shared harness.
  *Accept:* `go test ./...` runs meaningful tests.
  *Done:* new `testutil` package with `NewDB` (a temp-file DB, not `:memory:`; see §8),
  `SignToken`, `NewRSAKey` (hex PKIX DER, as the API expects), `SignRSA` (PKCS #1 v1.5 /
  SHA-256, hex), and `JWTSecret`. Added `middleware.WithEmail`, which the middleware now uses
  and handler tests use to fake an authenticated request. The middleware tests use the shared
  helpers. `users/users_test.go`: the valid registration, 10 rejection cases, and `KeyRegCreate`
  end to end against a real DB (stored on success, 400 and nothing stored on a bad signature).
  One test is skipped: a non-RSA public key panics. Confirmed by running it without the
  skip; fixed in T4.1. The `chain` and `wallets` packages get tests in later tasks (D11's fake
  Ethereum client lands with T4.4).
- [x] **T1.5 Switch the dev chain to Anvil (D10).** Set the default `DELSIGN_CHAIN_ID` to
  31337 and add a `scripts/smoke.sh` that runs register key → create wallet → create address
  → fund it from an Anvil account → send → check the balance. Funding uses the node's
  unlocked account (see §8), not a raw-SQL seed. Update the readme's dev chain instructions.
  *Accept:* the smoke script passes against `anvil` with the code still on its legacy
  transaction type. Later tasks reuse this script as the "manual smoke test" gate.
  *Done:* default chain ID 31337; the config override test now uses 1337, so it still
  differs from the default. `scripts/smoke.sh` builds the server, starts it on a temp DB
  (port 3099, `SMOKE_PORT`), with a random JWT secret and the chain ID read from the node.
  It mints an HS256 token and an RSA key with `openssl`, then runs keyreg → wallet → two
  addresses → fund source with 1 ETH from `eth_accounts[0]` → send 0.1 ETH through the API
  → wait for receipts → dest balance is exactly 0.1 ETH. It needs only `go`, `curl`, `jq`,
  `openssl`. Passed on anvil 1.8.6 (twice on the same chain, and on `--chain-id 1337`); the
  send was a type `0x0` (legacy) transaction. Fails cleanly when the node is down or the
  port is taken. Readme: new Dev chain and Smoke test sections; the Ganache raw-SQL seed is
  gone (its key isn't funded on Anvil, and T3.2 would break it), replaced by a
  `cast send --unlocked` funding example.

### Phase 2 — Dependency true-up

- [x] **T2.1 JWT v5 (D7).** Replace `golang-jwt/jwt` v3. *Accept:* tests cover a wrong
  algorithm, an expired token and a missing `exp` (all → 403).
  *Done:* v5.3.1. `authz.ValidateToken` uses `WithValidMethods(["HS256"])` and
  `WithExpirationRequired()`, and returns `jwt.MapClaims` directly, so the middleware's
  type assertion is gone. New middleware tests: missing `exp`, expired, HS384, and `none`,
  all 403. The existing test tokens now carry `exp`. Readme documents the token rules.
  **Breaking for the token issuer:** tokens must include `exp`.
- [x] **T2.2 Remove gddo.** Use `mime.ParseMediaType` in `helpers/parse.go`.
  *Accept:* gddo no longer in `go.mod`; tests for the Content-Type handling.
  *Done:* a Content-Type that doesn't parse is now rejected (415); gddo ignored the parse
  error. `helpers/parse_test.go`: 8 Content-Type cases (no header, parameters, mixed case,
  look-alike type, malformed) and 7 bad-body cases (empty, syntax, truncated, wrong type,
  unknown field, two objects, over 1 MB → 413).
- [ ] **T2.3 Move to the standard library `ServeMux`; harden the server (D8).** Rewrite the
  routes in `main.go` as method+path patterns on one `http.ServeMux`, and switch handlers
  from `mux.Vars` to `r.PathValue`. Add logging and recovery middleware, and apply the auth
  middleware to each protected route. Run `http.Server` with `ReadHeaderTimeout`,
  `ReadTimeout`, `WriteTimeout` and `IdleTimeout`, with graceful shutdown that closes the DB.
  *Accept:* neither gorilla/mux nor negroni in `go.mod`; route tests show each endpoint
  answers the right method, gives 405 for a wrong method, and gives 403 without a token
  (except public routes, if any are left); Ctrl-C shuts down cleanly.
  *Done:* `NewRouter(secret)` in `main.go` registers the 9 routes on one `ServeMux`;
  `GET /api/v1/users/{email}` stays public until T3.5. The auth middleware is now
  `func(http.Handler) http.Handler`. New `middleware/server.go` has `Logging` (method, path,
  status, duration) and `Recovery` (500 and a logged stack; re-panics
  `http.ErrAbortHandler`). Timeouts: read header 5s, read 10s, write 30s (send waits on the
  node), idle 120s. `signal.NotifyContext` (SIGINT, SIGTERM) → `Shutdown` with a 10s limit
  → DB closed. `main_test.go`: each route reaches its handler with a token, gets 405 for a
  wrong method, 403 without a token (protected routes), and an unknown path gets 404.
  Verified by hand: SIGINT and SIGTERM both log server and DB shutdown and exit 0; a bind
  error still closes the DB and exits 1. negroni's static file handler is dropped
  (there is no `public/` directory).
- [x] **T2.4 go-ethereum v1.17.x.** Upgrade only, keeping the current transaction type.
  Fix anything that breaks at compile time. *Accept:* manual smoke test passes.
  *Done:* v1.13.2 → v1.17.10 with no code changes; the APIs we use (`ethclient`,
  `types.NewTransaction`, `NewEIP155Signer`, `crypto`) all still compile. `scripts/smoke.sh`
  passes against anvil 1.8.6.
- [x] **T2.5 Remaining bumps.** go-sqlite3 and the transitive modules; `go mod tidy`.
  *Accept:* `govulncheck` shows 0 reachable findings, or each one left is listed here with a reason.
  *Done:* go-sqlite3 v1.14.17 → v1.14.52 (SQLite 3.42.0 → 3.53.4), then `go get -u ./...`
  and `go mod tidy`. That fixed the last reachable finding (gorilla/websocket v1.4.2 →
  v1.5.3, GO-2026-6278, reached via `ethclient`). Exception: golang-set/v2 is held at
  v2.6.0, go-ethereum's version, because v2.9.0 pulls `go.mongodb.org/mongo-driver/bson`
  into the build. `go.mod` now has 3 direct dependencies (go-ethereum, jwt/v5, go-sqlite3).
  govulncheck v1.9.0: **0 called / 0 imported / 0 required** (baseline 24 / 7 / 27).
  `go list -m -u all` still shows 98 of 190 modules with updates; these are in the module
  graph but not in our build (mostly go-ethereum's own tooling and test dependencies).

### Phase 3 — Security fixes

- [ ] **T3.1 Stop logging secrets.** Remove the logging of the private key
  (`wallets/address.go`), the auth header (`middleware/authcheck.go`), the claims, and the
  full key-registration struct. *Accept:* a grep for these log calls finds nothing.
- [ ] **T3.2 Encrypt private keys at rest (D3).** Encrypt and decrypt in `AddressDB`,
  and add the `import-key` subcommand. Document it in the readme (the raw-SQL seed was removed in T1.5).
  *Accept:* new rows start with `enc:v1:`; reading a plaintext row returns an error; an Anvil
  account imported with `import-key` can send; the smoke script still passes; tests round-trip a key through encryption and
  show that a wrong encryption key fails.
- [ ] **T3.3 Key rotation needs the current key (D5).** *Accept:* tests show the first
  registration succeeds, a replacement without a current-key signature fails (403), and a
  replacement with one succeeds.
- [ ] **T3.4 Replay protection (D4).** Add the new payload fields, the expiry check and the
  `used_nonces` table. Document the new signing format in the readme. *Accept:* tests show a
  replayed request → 409 and an expired request → 400.
- [ ] **T3.5 Lock down user lookup (D6).** *Accept:* tests show your own email → 200, another
  user's email → 403, and no token → 403.

### Phase 4 — Robustness fixes

- [ ] **T4.1 Handle every error.** Check the address insert in `CreateAddressForWallet`, the
  `SuggestGasPrice` error, the public-key parse in `chain.validateSignature`, and the
  signature hex decode in `validateKeyReg`. Check the public-key types (assert with `ok`).
  *Accept:* a test for each malformed input gets a 4xx response, not a 500 or a panic.
  Remove the skip on `TestValidateKeyRegRejectsNonRSAKey` (added in T1.4; currently panics).
- [ ] **T4.2 Input validation.** Check addresses with `common.IsHexAddress` and normalize
  them to checksum form before DB lookups; require `amount` > 0; decode the send body with
  `DecodeJSONBody`. *Accept:* a lowercase source address that the user owns is accepted;
  invalid input → 400.
- [ ] **T4.3 Status codes.** GET handlers return 200 and the balance response sets a JSON
  Content-Type. Decode failures → 400 instead of 500. Ownership failures → 403 or 404
  (agreed in review). *Accept:* tests check the status codes.
- [ ] **T4.4 EIP-1559 transactions (D9).** Switch `SendEth` to `DynamicFeeTx` behind the
  chain interface. *Accept:* manual smoke test passes; a unit test with a fake client checks
  the fields of the transaction it builds.
- [ ] **T4.5 Concurrent sends from one address.** Serialize sends per source address with an
  in-process lock, so two parallel requests don't get the same nonce. *Accept:* the test fires
  two concurrent sends and gets two different nonces.

### Phase 5 — Close-out

- [ ] **T5.1 Docs.** Update the readme: config variables, how the DB is created now, the new
  signing format, the smoke-test script, and the Anvil setup.
- [ ] **T5.2 Roadmap.** Create `plan/roadmap.md` listing known limitations and deferred work,
  each with a short reason and a pointer to where it came from. At least: key recovery and
  rotation without the current key (D5); the items marked out of scope in §1 (HSM/KMS custody,
  rate limiting, migrations framework, address book, ERC-20); and anything recorded in §8
  during execution. *Accept:* every deferred item in this plan appears in the roadmap.
- [ ] **T5.3 Final verification.** Run the full gates and `govulncheck` again, and compare
  with the baseline from T0.1. Record the result in §8.

## 8. Open questions and notes

_No open questions._

**Decision log**
- 2026-10-23: D3 agreed: AES-GCM with a key from an environment variable; plaintext keys not supported.
- 2026-10-23: D4 agreed: hard cutover to the new signed format.
- 2026-10-23: D6 agreed: users may look up only their own record.
- 2026-10-23: D8 agreed: standard library `ServeMux`; gorilla/mux and negroni removed.
  Reason: negroni isn't deprecated, but it adds little here (logging and recovery, plus an
  unused static file handler), and since Go 1.22 `ServeMux` covers all of mux's routing that we use.
- 2026-10-23: D5 agreed. A lost key means lockout, which is accepted for the PoC; it goes in the roadmap (T5.2).
- 2026-10-23: D10 changed from Ganache to Anvil (Ganache was sunset); T1.5 added.
- 2026-10-23: D1, D2, D7, D9, D11, D12 agreed as proposed.
- 2026-10-23: Planning convention: `plan/` holds one file per plan (`NN-<name>.md`), plus a
  `roadmap.md` that is kept up to date.
- 2026-10-23: Plan agreed. The user does all git commits.
- 2026-10-23: D12 changed from Go 1.26.10 to Go 1.27.3. Go 1.27 is the current major
  release, and 1.26 loses support when 1.28 ships. The baseline (Go 1.26.5) stays as recorded.
- 2026-10-23 (T1.1): D1 refined. `DELSIGN_JWT_SECRET` must be at least 32 bytes, so the
  external token issuer has to change its secret from `"secret"` to match.
  `DELSIGN_KEY_ENC_KEY` is added in T3.2, where it is first used.
- 2026-10-26 (T1.2): SQLite foreign keys are declared in the schema but **not enforced**
  (`PRAGMA foreign_keys` is off by default), so wallets can be created for emails with no
  `users` row. Left as is because turning it on changes behaviour. → roadmap candidate.
- 2026-10-26 (T1.2): Note for T1.4: `:memory:` SQLite gives each pooled connection its own
  empty database. Test helpers must use a temp file or `SetMaxOpenConns(1)`.
- 2026-10-26 (T1.5): **T1.5 blocked.** The prebuilt Foundry binaries (`anvil`, `cast`) need
  glibc ≥ 2.32–2.34; the WSL distro is Ubuntu 20.04 (glibc 2.31). The user chose to upgrade
  WSL to Ubuntu 24.04 before continuing. Afterwards, check `ldd --version` (≥ 2.39) and run
  `foundryup` and `anvil --version`, then resume T1.5.
- 2026-10-27 (T1.5): **Agreed:** make `scripts/smoke.sh` work with
  any dev node. Fund the test address via `eth_sendTransaction` from one of the node's own
  unlocked accounts (`eth_accounts[0]`), and read the chain ID with `eth_chainId`, instead of
  seeding an Anvil private key with raw SQL. That removes the script's dependence on DB
  internals (it survives T3.2's encryption), and the script also runs on Hardhat or Ganache.
- 2026-10-27 (T1.5): Blocker cleared: WSL is on Ubuntu 24.04 (glibc 2.39), anvil 1.8.6.
- 2026-10-27 (Phase 2): At the user's request, T2.1–T2.5 were done together as **one
  change** instead of one commit per task (§6). They share `go.mod`/`go.sum` and the
  middleware, so splitting them afterwards isn't worth it.
- 2026-10-27 (T2.5): Transitive bumps beyond go-ethereum's own pins are taken only where
  they don't grow the build; golang-set/v2 stays on v2.6.0 (see T2.5). Re-check at T5.3.
- 2026-10-27 (T2.3): Existing handler status codes are still wrong in the places T4.3
  lists: user lookup of an unknown email → 500, send with a bad body → 500, `GET /wallets`
  → 201. Seen in the route tests; left for T4.1/T4.3.
