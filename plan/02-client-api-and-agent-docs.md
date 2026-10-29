# 02 — Client-Safe API and Agent-Consumable Docs

- **Status:** Agreed 2026-10-29. In progress.
- **Branch:** `feature/refresh`
- **Created:** 2026-10-29

## 1. Purpose and scope

A frontend project (a separate repo) is about to be updated to call this backend, with
Claude doing much of that work. This plan does two things:
1. Fix the API problems a JavaScript client would hit, before the frontend is written against the API.
2. Produce documentation that an agent working in the frontend repo can use without
   access to this repo's code. It should be precise enough that the agent gets signing
   right the first time, and checked by tests here so it can't drift from the code.

A small MCP server over the same docs was considered as a way to learn the pattern an
enterprise with many APIs might use. It is deferred to the roadmap (§5, D7).

**In scope**
- Wei amounts as decimal strings (D1).
- Empty lists returned as `[]` (D2).
- An OpenAPI 3.1 spec, a client guide, signing test vectors, and a way to copy them into
  the client repo (D3–D6).

**Out of scope**
- Frontend changes. The frontend repo gets the docs; its own plan does the work.
- Other roadmap items (JWT `iss`/`aud`, rate limiting, …).
- An MCP server (D7, deferred to the roadmap).
- Generating a TypeScript client. The spec makes that possible, but it's the frontend's choice.

## 2. The problem with numbers

`amount` is a JSON number in wei, both in `POST /wallets/send` and in the balance
response. JavaScript numbers are doubles and represent integers exactly only up to
2⁵³ − 1 (about 9 × 10¹⁵ wei, or 0.009 ETH). So in a browser:
- `JSON.parse` of a balance of 1 ETH (`1000000000000000000`) can silently give a different
  number. Here it happens to round-trip, but most values don't.
- `JSON.stringify` can't send an amount larger than that exactly.
- The send signature covers the amount's decimal digits. If the client signs the digits it
  meant but sends a rounded number, the server sees a different message and returns 401.
  That's a confusing failure to debug.

Strings are what Ethereum JSON-RPC and most wallet APIs use, for this reason.

## 3. Key design decisions

| ID | Decision | Proposal | Status |
|----|----------|----------|--------|
| D1 | Amount format | `amount` is a JSON **string** holding a base-10 integer in wei, in the send request and the balance response. Send amounts must match `^[1-9][0-9]*$` (no sign, leading zeros, spaces, decimals or exponent) and fit in 256 bits. Balances may be `"0"`. Because the form is canonical, the signed message is unchanged: it contains the same digits as today. **Hard cutover**, as in plan 01's D4: a JSON number gets a 400 that says to send a string. | Agreed |
| D2 | Empty lists | `GET /wallets` and `GET /walletctx` return `[]`, not `null`, when there's nothing to list. It's a breaking change, but D1 already breaks the client, so it goes in the same release. Removes that item from the roadmap. | Agreed |
| D3 | Docs format and location | `docs/api/` in this repo is the source of truth. It holds: `openapi.yaml` (OpenAPI 3.1: all endpoints, schemas, every status code, `additionalProperties: false` on request bodies), `client-guide.md` (signing and flows, in browser terms), `test-vectors.json`, and `README.md` (what's there, plus a paragraph for the client's `CLAUDE.md`). The guide holds what OpenAPI can't express: exactly which strings get signed, key formats, request ids and expiry, and what each error means for the UI. | Agreed |
| D4 | Signing test vectors | A fixed RSA test key, committed under `docs/api/` and clearly marked test-only. For key registration, rotation and send: the inputs, the exact message string, and the expected hex signature. PKCS #1 v1.5 signatures are deterministic, so a client can check its signing code against them without a server. | Agreed |
| D5 | Keeping docs true | Go tests here fail when the docs drift: (a) the routes in the spec match the routes the server registers, by method and path; (b) the example bodies in the spec decode into the Go request structs with unknown fields disallowed; (c) the vectors verify with the server's own verification code and match what a generator makes. The spec is YAML, read with `gopkg.in/yaml.v3`. That module is already in the module graph via go-ethereum, so this adds no new module. (a) needs `NewRouter` to register routes from a table the test can read. | Agreed |
| D6 | Getting docs to the client | `scripts/export-client-docs.sh <client-repo>` copies `docs/api/` to `<client-repo>/docs/delsign-api/` and writes `VERSION` (backend commit and date). The client's `CLAUDE.md` points there. Copies are versioned with the client code, so an agent always sees the API version that code targets. | Agreed |
| D7 | MCP server (learning) | `tools/apidocs-mcp/`, a **separate Go module**, so the MCP SDK stays out of the backend's dependency graph. Uses the official `github.com/modelcontextprotocol/go-sdk` (v1.8.0 at the time of writing), over stdio. **Read-only, docs only:** it serves the same `docs/api/` files and never calls the live API. An agent that can call `send` could move funds, and the user's RSA key belongs in the browser, not an agent's tool. It reads the docs directory from a `-docs` flag, because `go:embed` can't reach outside the module's directory. | Deferred (roadmap) |

## 4. What the client agent gets

| File | Answers |
|---|---|
| `openapi.yaml` | Which endpoints exist, the exact request and response shapes, and each status code |
| `client-guide.md` | How to make a key in WebCrypto and export it (`spki` → hex), the exact strings to sign, `crypto.randomUUID()` and `expiry`, a full send in TypeScript, how to handle each error, wei amounts as `BigInt` strings |
| `test-vectors.json` | Whether my signing code is right (a unit test in the frontend) |
| `README.md` | What to read first; the version these docs describe |

The guide's TypeScript snippets can't be run here (there's no Node in this environment).
They're written to the test vectors, and the frontend's first task should be a unit test
that runs them against those vectors.

## 5. The MCP server, and when the pattern is worth it

For one backend and one client, plain files are better than MCP: they need no
infrastructure, every agent can read them, they sit in git next to the code that uses them,
and the version matches by construction. So this plan stops at files. The MCP server is
kept on the roadmap as a way to learn the pattern; this section records the design for when
it's picked up.

MCP is worth it when there are **many APIs**. Then the problem is finding the right API and
loading only what you need, not reading one spec. An "API catalog" MCP server in front of
an organization's spec registry typically offers:
- `search_apis(query)`: which API and operation does what I need?
- `get_operation(api, operationId)`: one operation with its schemas resolved, rather than
  loading a 5,000-line spec into context;
- `get_guide(api, topic)` and `get_examples(api, operationId)`: the parts OpenAPI can't express;
- optionally, calls against a **sandbox** environment with the agent's own test credentials.

Central control is the other benefit: one server enforces which APIs an agent may see and
always serves the current published version. The costs are a service to run, version
skew (the server serves the latest version, while the client may target an older one), and
weaker reviewability than files in the repo.

A server here would use the same tool shapes on a small scale: `list_operations`,
`get_operation(operationId)`, `get_signing_guide`, `get_test_vectors`, with the files also
exposed as MCP resources. To compare it directly with the plain files, point an agent in a
scratch client at each one and ask it to write the send flow.

## 6. Execution approach

As in plan 01:
- Claude makes each change, runs the gates, and ticks §7. The user reviews and commits.
- Gates: `go build ./...`, `go vet ./...`, `go test ./...`. Phase 1 also runs
  `scripts/smoke.sh` against Anvil.
- Phases may be done in one go.

## 7. Task list

Legend: `[ ]` todo · `[~]` in progress · `[x]` done

### Phase 1 — API change

- [x] **T1.1 Amounts as strings (D1).** Send request and balance response. Parse with
  `big.Int.SetString(s, 10)` after a regexp check. *Accept:* tests cover a valid
  amount, zero, a negative amount, leading zeros, an exponent, a decimal, a JSON number
  (400 that says to use a string), and an amount over 2²⁵⁶ − 1. The balance is
  `{"address": "0x…", "amount": "1000…"}`. The signed message has the same digits as before.
  *Done:* new `chain.Wei` type (`chain/wei.go`) wrapping `*big.Int`, used by `SendPayload`
  and `Balance`. Its `UnmarshalJSON` accepts only `^(0|[1-9][0-9]*)$` strings up to 256 bits;
  `checkFields` still requires a positive amount. Its error message reaches the client
  through `DecodeJSONBody`'s default case: `Request body could not be decoded: amount must
  be a JSON string of decimal digits (wei), e.g. "1000"`. `sendMessage` now formats the
  amount with `%s`. Tests: `TestWeiJSON` (round trip up to 2²⁵⁶ − 1, the message for a
  JSON number) and 9 new invalid-amount cases in `TestSendRejectsInvalidFields`.
- [x] **T1.2 Empty lists (D2).** *Accept:* tests show a user with no wallets gets `[]`
  from both endpoints.
  *Done:* `ListWallets` and `WalletsAndAddressesForUser` start from empty slices.
  `TestEmptyListsAreEmptyArrays` covers no wallets (both endpoints) and a wallet with no
  addresses; it fails with `null` when the fix is reverted.
- [x] **T1.3 Update the smoke script, readme and roadmap.** Smoke and the readme's curl
  examples send string amounts. In the roadmap, add D1 and D2 to the frontend's breaking
  changes and remove the `null` item. *Accept:* smoke passes; the curl examples run as written.
  *Done:* smoke `send_body` uses `--arg` for the amount. The readme's signing section, endpoint
  table, 400 row and curl send example are updated. Smoke PASS on Anvil. The readme curl
  blocks ran against a fresh server: balance `"1000000000000000000"`, send OK, and the
  destination gets `"1000"`. A new user gets `[]` from both list endpoints, and a JSON-number
  amount gets the 400 above.

### Phase 2 — Client docs

- [x] **T2.1 OpenAPI spec (D3) + route drift test (D5 a, b).** Move the route list into a
  table that `NewRouter` uses. *Accept:* the spec validates as OpenAPI 3.1 (checked once
  with a linter, recorded here); the drift tests pass, and fail when a route is added to the
  table but not the spec.
  *Done:* `docs/api/openapi.yaml` covers all 9 operations, with every status code each
  handler can return, shared `components.responses` for auth, 413, 415, 500 and 502, and
  `additionalProperties: false` on every object schema. `main.go` now registers routes from
  `apiRoutes`. `apidocs_test.go` checks:
  - routes match the spec (`TestOpenAPIMatchesRoutes`);
  - each object schema's properties and `required` match its Go type's JSON fields and
    omitempty (`TestOpenAPISchemasMatchGoTypes`);
  - request and response examples decode into the Go types with unknown fields disallowed
    (`TestOpenAPIExamplesDecode`);
  - every `$ref` resolves (`TestOpenAPIRefsResolve`);
  - the keyreg and send request examples are the test-vector bodies
    (`TestOpenAPIExamplesAreVectors`).

  Each was shown to fail by breaking it on purpose: an extra route, a renamed property, an
  unknown example field, a broken `$ref`. Small refactors for the tests: `chain.SendMessage`
  is exported, `users.RotationMessage` is added (and used by `validateRotation`), and the
  send response is a named type, `chain.SendResult`. `gopkg.in/yaml.v3` v3.0.1 is now a
  direct requirement; it was already in the graph, which is still 184 modules.
  **Linter run outstanding:** running vacuum (`go run github.com/daveshanley/vacuum@v0.36.0
  lint -d docs/api/openapi.yaml`) was blocked by this session's permissions, so the user
  runs it. Until then, the structural checks above stand in.
- [x] **T2.2 Test vectors (D4, D5 c).** Generator plus a test that checks them.
  *Accept:* the test fails if a vector is edited by hand; vectors verify with
  `users.VerifySignature` and the send message builder.
  *Done:* `docs/api/test-vectors.json` has two published test keys (SPKI and PKCS #8 hex) and
  5 vectors: register, rotate, send, send with lowercase addresses (a different signature),
  and send with an amount over 2⁵³. `go test -run TestVectors -update-vectors .` regenerates
  them, keeping the keys; regeneration gives identical bytes. `TestVectors` fails on any
  difference from the generator's output (checked by editing one character). It also checks
  that each signature is in the body, verifies with `users.VerifySignature`, and signs the
  message the server rebuilds from the decoded body. openssl reproduces all 6 signatures
  and both public keys from the published keys, so a second implementation agrees.
- [x] **T2.3 Client guide.** In browser terms: WebCrypto keys, the message strings,
  request id and expiry, full flows (register, rotate, send), and errors. *Accept:* every
  value in its examples comes from the test vectors, and it tells the frontend to
  test against them first.
  *Done:* `docs/api/client-guide.md` covers: how it works; connecting (same origin only, see
  §8); the encoding rules; WebCrypto key generation, export, signing and storage, with the
  lost-key warning; register, rotate, wallets, balance (BigInt) and send flows; an error table
  with what the UI should do; retry rules for sends; a Vitest test against the vectors; and
  the breaking changes since the old API. Example values match the vectors, and the
  send/rotate messages in the Vitest example are checked against them. The TypeScript was
  not run (there's no JS runtime here); the vector test is the frontend's first check.
- [x] **T2.4 Index and export (D6).** `docs/api/README.md`, `scripts/export-client-docs.sh`, a
  pointer from the readme. *Accept:* exporting to a temp directory gives the files plus `VERSION`.
  *Done:* `docs/api/README.md` has the file index and a ready-to-paste `CLAUDE.md` section for
  client repos. `scripts/export-client-docs.sh <client-repo> [subdir]` replaces
  `<client-repo>/docs/delsign-api/` (a stale file was removed in the test), copies the docs,
  and writes `VERSION` with the commit, branch and time. It marks the commit "plus
  uncommitted changes" when `docs/api` is dirty, and rejects bad arguments. The readme's API
  section points to `docs/api/`, and its layout list includes it.

### Phase 3 — Close-out

- [ ] **T3.1 Roadmap and gates.** Update the roadmap; full gates; record results here.

## 8. Open questions and notes

**Open questions**
- _None._

**Decision log**
- 2026-10-29: Q1 resolved: error bodies stay plain text. The spec documents them as
  `text/plain`; JSON errors with a stable `code` go on the roadmap.
- 2026-10-29: Q2 resolved: work continues on `feature/refresh`.
- 2026-10-29: Q3 resolved: stop after the client docs. The MCP server (D7, old Phase 3) is
  deferred to the roadmap, and close-out becomes Phase 3.
- 2026-10-29: D1–D6 agreed as proposed.
- 2026-10-29 (Phase 2, done in one go): found while writing the guide, and added to the
  roadmap rather than fixed here:
  - **No CORS.** A preflight `OPTIONS` gets a 405 with no CORS headers (checked against the
    router), so browser clients need a same-origin proxy. The guide says so.
  - **Replays can't report the outcome.** The `requestId` is recorded before the broadcast,
    so a retry after a lost response gets a 409 without the `txnid`, and a 502 uses the id up.
    The guide's retry rules are written around this.
  - Also fixed in the spec: `GET /wallets` has no `ORDER BY`, so the spec says "in no
    particular order".
- 2026-10-29 (Phase 1, done in one go): D1 is implemented as a `Wei` JSON type rather than
  a string field parsed in the handler. Decoding then rejects bad amounts, and the
  error can say "send a string". `go test -race ./...` passes.
