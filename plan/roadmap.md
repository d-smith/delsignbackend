# Roadmap: known limitations and deferred work

What `delsignbackend` deliberately doesn't do yet, and why. Each item names where it came from:
a decision (D*n*) or task (T*n.n*) in [01-true-up-plan.md](01-true-up-plan.md), the plan's
§1 out-of-scope list, or the close-out review.

Keep this file current. When a plan defers something, add it here; when a later plan picks
an item up, move it to that plan and delete it here.

_Last updated: 2026-10-29, at the close of plan 02._


## Security and key custody

- **No recovery for a lost RSA key.** Replacing a key needs a signature from the current key
  (D5), so a user who loses theirs is locked out of their funds for good. A recovery path
  (e.g. an admin-approved reset with a delay, or a pre-registered backup key) needs its own
  design, because done carelessly it reopens the "stolen JWT replaces the key" hole that D5
  closed. *Source: D5, T3.3.*
- **Keys are encrypted with an environment variable, not an HSM or KMS.** Anyone with both
  the database and `DELSIGN_KEY_ENC_KEY` has every private key. *Source: §1 (out of scope), D3.*
- **No rotation of `DELSIGN_KEY_ENC_KEY`.** There is one key, with no way to re-encrypt
  existing rows under a new one. The `enc:v1:` prefix leaves room for a versioned scheme
  (e.g. `enc:v2:<key id>:…`) plus a re-encrypt command. *Source: close-out review of T3.2.*
- **JWTs: shared-secret HS256, no issuer or audience checks.** Any token signed with
  the secret is accepted, whatever its `iss` or `aud`. Asymmetric tokens (RS256/EdDSA with
  the issuer's public key or JWKS) would also remove the shared secret.
  *Source: close-out review of D7/T2.1.*
- **No rate limiting.** Nothing slows down signature guessing or request floods.
  *Source: §1 (out of scope).*
- **No multi-tenant isolation.** One database, one encryption key, one JWT secret for
  everyone. *Source: §1 (out of scope).*
- **Logs are unstructured and include emails.** T3.1 removed secrets from the logs, but per-request
  debug lines (`check auth header`, `email from JWT …`) remain, and emails are personal
  data. Move to `log/slog` with levels, and decide what may be logged. *Source: T3.1, close-out review.*


## Running more than one instance

- **Send serialization is in-process.** The per-address lock (T4.5) only covers one server
  process; two instances could still give two sends the same account nonce. Needs a shared
  lock or a nonce manager (e.g. in the database). The lock map also never shrinks, which is
  fine at PoC scale. *Source: T4.5.*
- **Replay protection is per database.** `used_request_ids` works across instances only if
  they share the database. *Source: T3.4.*


## Data

- **No migrations framework; SQLite only.** The schema is applied with
  `CREATE TABLE IF NOT EXISTS`, so changing an existing table means a new database file.
  *Source: §1 (out of scope), T1.2.*
- **Foreign keys aren't enforced.** They're declared in the schema, but
  `PRAGMA foreign_keys` is off, so wallets can exist for emails with no `users` row. Turning it on
  changes behaviour (wallet creation before key registration would fail), so it needs a
  decision first. *Source: T1.2.*


## Ethereum

- **Plain ETH transfers only.** Gas is fixed at 21000 and there is no `EstimateGas`, so
  sending to a contract that needs more gas fails. ERC-20 or contract calls need gas
  estimation and calldata. *Source: §1 (out of scope), T4.4.*
- **Fee policy is a fixed formula.** Fee cap = 2 × latest base fee + the node's suggested
  tip. There's no tuning, no fee bump for a transaction that is stuck, and no tracking of
  pending transactions. *Source: T4.4.*


## API and protocol

- **Frontend must catch up.** These are breaking changes for the client:
  - tokens need `exp` (T2.1);
  - sends use the new signed format with `requestId` and `expiry` (D4, T3.4);
  - key replacement needs `sig4rotate` (T3.3);
  - `GET /users/{email}` needs a token (T3.5);
  - request bodies with unknown fields get a 400 (T4.2);
  - ownership failures are 403 and node failures are 502 (T4.3);
  - wei amounts are decimal strings, in sends and balances (plan 02, D1);
  - empty lists are `[]`, not `null` (plan 02, D2).

  The frontend is outside this repo. `docs/api/client-guide.md` lists these changes for
  client developers. *Source: §1, D4; plan 02.*
- **No CORS support.** The server sends no CORS headers and answers preflight `OPTIONS`
  requests with a 405, so a browser app must reach it from the same origin (a dev-server
  proxy, or a reverse proxy in production). The client guide says so. Adding CORS means
  an allowed-origins setting and handling `OPTIONS` before auth. *Source: plan 02, T2.3.*
- **A replayed send can't report what happened.** The `requestId` is recorded before the
  broadcast, so a client retrying after a lost response gets a 409 without the original
  `txnid`, and after a 502 the id is used up. Storing the outcome with the request id, and
  returning it on a replay, would make retries idempotent. *Source: plan 02, T2.3.*
- **No clock-skew allowance on `expiry`.** A request is accepted only if
  `now < expiry ≤ now + 5 min`; the readme tells clients to use 1–2 minutes. Relax it only
  if clients see rejections in practice. *Source: T3.4.*
- **Error bodies are plain text.** A JSON body (`{"error": "…", "code": "…"}`) would give
  clients a stable code to switch on instead of matching message text. It's a breaking
  change, deferred when plan 02 was drafted. *Source: plan 02, Q1.*
- **No MCP server for the API docs.** Plan 02 ships `docs/api/` as files that are copied into the
  client repo. A read-only MCP server over those files (its own Go module, the official Go
  SDK, docs only and never live API calls) was designed but deferred; it's mainly
  useful for learning the "API catalog" pattern used with many APIs.
  *Source: plan 02, §5 and D7.*
- **No address book.** Users can't save or look up other people's addresses.
  *Source: §1 (out of scope).*
- **`import-key` needs the full configuration**, including `DELSIGN_JWT_SECRET`, which
  it doesn't use. *Source: T3.2.*


## Dependencies

- **golang-set/v2 is held at v2.6.0**, go-ethereum's version, because v2.9.0 pulls
  `mongo-driver/bson` into the build. Revisit when go-ethereum moves. *Source: T2.5.*
- **Modules with updates available (98 of 184 at close-out)** are in the module graph but not in
  the build (mostly go-ethereum's tooling and test dependencies), so they don't affect the
  binary. Re-run `govulncheck` regularly; the result at close-out is in plan 01, T5.3.
  *Source: T2.5, T5.3.*
