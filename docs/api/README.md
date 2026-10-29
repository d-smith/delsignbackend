# Delsign API docs for client developers

The contract for clients of the delsign backend, written to be read by people and by
coding agents. The source of truth is `docs/api/` in the delsignbackend repo. Tests there
check these files against the server code, so they can't silently drift. Client repos
get a copy with `scripts/export-client-docs.sh`, which adds a `VERSION` file naming the
backend commit.

| File | Read it for |
|---|---|
| `client-guide.md` | **Start here.** Connecting, key handling and signing in WebCrypto, every flow, errors and retries |
| `openapi.yaml` | Exact endpoints, request and response schemas, status codes (OpenAPI 3.1) |
| `test-vectors.json` | Signatures your signing code must reproduce; the guide has a ready-made test |
| `VERSION` | (In copies only.) The backend commit these docs describe |

Don't edit copies by hand; export again from the backend.

## For coding agents in a client repo

Add this to the client repo's `CLAUDE.md` (or `AGENTS.md`), adjusting the path:

```markdown
## Delsign backend API

The API contract is in `docs/delsign-api/`, copied from the delsignbackend repo (see `VERSION`).
- Read `docs/delsign-api/client-guide.md` before writing or changing code that calls the API.
  Use `openapi.yaml` for exact shapes and status codes.
- Wei amounts are decimal strings on the wire and `bigint` in code, never `number`.
- Signing code must pass a test against `test-vectors.json` (the guide has one). Sends sign
  the request's exact strings, so don't change them after signing.
- Don't edit `docs/delsign-api/`; ask for a fresh export from the backend instead.
```
