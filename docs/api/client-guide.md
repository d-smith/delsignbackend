# Delsign API: client guide

How to call the delsign backend from a browser app. `openapi.yaml` is the contract for
endpoints and shapes; this guide covers what the spec can't: the exact strings to sign, key
handling in WebCrypto, request ids and expiry, and what each error means for the UI.
`test-vectors.json` has worked signatures. Make your signing code reproduce them before
anything else (see [Test against the vectors first](#test-against-the-vectors-first)).

The examples are TypeScript using only browser APIs (`fetch`, `crypto.subtle`,
`crypto.randomUUID`). They weren't run in the backend repo, where there's no JavaScript
runtime; the vector test below is how you check them.


## How it works

The server holds an Ethereum key for each of the user's addresses. It signs and sends a
transaction only when the request carries a signature from the user's **RSA key**, which
lives in the browser. A JWT from the identity provider authenticates every call, but it is
never enough on its own to move funds or replace the RSA key.

1. The user logs in with the identity provider and gets a JWT with an `email` claim.
2. The app makes an RSA key pair in the browser and registers its public key.
3. The user creates wallets, and addresses in them.
4. To send ETH, the app signs the request with the RSA private key.


## Connecting

- **Base URL:** `http://localhost:3010` in development; all paths start `/api/v1/`.
- **Same origin only.** The server sends no CORS headers, and the browser's preflight
  request for the `Authorization` header gets a 405. Serve the API from the app's origin:
  in development, proxy `/api` through the dev server (e.g. Vite `server.proxy:
  { "/api": "http://localhost:3010" }`); in production, use a reverse proxy.
- **Auth:** every request sends `Authorization: Bearer <jwt>`. The app never makes tokens; it
  passes on the identity provider's. The token must have `exp`. Any token problem gets a
  **403**, so treat 403 from `GET /api/v1/authping` as "log in again".
- **Bodies:** `Content-Type: application/json`, with exactly the documented fields. An extra
  field gets a 400.
- **Errors** are `text/plain`. Switch on the status code; show or log the text, but don't
  parse it.


## Encoding rules

| Thing | Format |
|---|---|
| RSA public key (`pubkey`) | Hex of the SPKI (PKIX) DER encoding: what `crypto.subtle.exportKey("spki", …)` returns |
| Signatures (`sig4reg`, `sig4rotate`, `sig`) | RSASSA-PKCS1-v1_5 with SHA-256 over the UTF-8 message, hex |
| Wei amounts (`amount`) | Decimal **string**, e.g. `"1000000000000000000"`. Use `bigint` in code, never `number` |
| Addresses | `0x` + 40 hex digits, any letter case. Responses use checksum case |
| `requestId` | A UUID, from `crypto.randomUUID()` |
| `expiry` | Unix time in **seconds** (a JSON number), at most 300 seconds ahead |

Hex may be either case on the way in. `toHex` below gives lowercase, and `GET /users/{email}`
returns `pubkey` exactly as it was registered.


## Keys and signing

```ts
// delsignCrypto.ts
const RSA = { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" } as const;

export function toHex(buf: ArrayBuffer): string {
  return Array.from(new Uint8Array(buf), (b) => b.toString(16).padStart(2, "0")).join("");
}

export function fromHex(hex: string): ArrayBuffer {
  if (hex.length % 2 !== 0 || /[^0-9a-fA-F]/.test(hex)) throw new Error("invalid hex");
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(2 * i, 2 * i + 2), 16);
  return out.buffer as ArrayBuffer;
}

/** A new signing key. The private key can't be exported; the public key always can. */
export function generateUserKey(): Promise<CryptoKeyPair> {
  return crypto.subtle.generateKey(
    { ...RSA, modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]) },
    false,
    ["sign", "verify"],
  );
}

/** The public key in the API's format: hex of the SPKI DER encoding. */
export async function publicKeyHex(key: CryptoKey): Promise<string> {
  return toHex(await crypto.subtle.exportKey("spki", key));
}

/** The API's signature format. WebCrypto hashes the message with SHA-256 itself. */
export async function signHex(privateKey: CryptoKey, message: string): Promise<string> {
  return toHex(await crypto.subtle.sign(RSA.name, privateKey, new TextEncoder().encode(message)));
}
```

**Keeping the key.** A `CryptoKeyPair` can be stored in IndexedDB as it is (for example with
`idb-keyval`'s `set("delsign-key", pair)`), and stays non-extractable there. There is
**no recovery for a lost key**: replacing a key needs a signature from the current one, so if
the user clears site data or changes browser, their funds are locked. Tell users this
in the UI. Making the key extractable so users can back it up is a product decision with
its own risks.

**Is the local key the registered one?** Compare `publicKeyHex(pair.publicKey)` with
`pubkey` from `GET /api/v1/users/{email}`. A mismatch means sends will get 401.


## Flows

A small wrapper used below:

```ts
export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function call<T>(token: string, method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  if (!res.ok) throw new ApiError(res.status, text.trim());
  return (text ? JSON.parse(text) : undefined) as T;
}
```

### Register a key (`POST /api/v1/keyreg`)

`sig4reg` is the new key's signature over the email, which must equal the token's
`email` claim. 200 with an empty body.

```ts
const pubkey = await publicKeyHex(pair.publicKey);
await call(token, "POST", "/api/v1/keyreg", {
  email,
  pubkey,
  sig4reg: await signHex(pair.privateKey, email),
});
```

To find out whether the user has registered, call `GET /api/v1/users/{email}`: 404 means
no key yet.

### Replace the key (same endpoint)

Also needs `sig4rotate`: the **current** key's signature over the email followed by the
**new** public key's hex, with no separator. Without it the request gets a 403.

```ts
const pubkey = await publicKeyHex(next.publicKey);
await call(token, "POST", "/api/v1/keyreg", {
  email,
  pubkey,
  sig4reg: await signHex(next.privateKey, email),
  sig4rotate: await signHex(current.privateKey, email + pubkey),
});
// Only now replace the stored key pair with `next`
```

### Wallets and addresses

| Call | Returns |
|---|---|
| `POST /api/v1/wallets` (no body) | 201 `{"id": 2}`. Ids are shared across all users; don't assume 1 |
| `GET /api/v1/wallets` | `[2, 5]`, or `[]` |
| `POST /api/v1/wallets/{id}/addresses` (no body) | 201 `{"eoa": "0x…"}`, a new, empty address |
| `GET /api/v1/walletctx` | `[{"walletId": 2, "address": "0x…"}, …]`, or `[]` |

### Balance (`GET /api/v1/wallets/balance/{address}`)

```ts
const { amount } = await call<{ address: string; amount: string }>(token, "GET", `/api/v1/wallets/balance/${address}`);
const wei = BigInt(amount);
```

To show ETH, use bigint arithmetic or a library such as viem (`formatEther(wei)`, and
`parseEther("0.1")` for input). Never go through `Number` or `parseFloat`.

### Send ETH (`POST /api/v1/wallets/send`)

The signed message is `source + dest + amount + requestId + expiry`: the five strings
concatenated with no separators, using **exactly the strings in the body** (`expiry` in
decimal).

```ts
export interface SendRequest {
  source: string;
  dest: string;
  amount: string;
  requestId: string;
  expiry: number;
  sig: string;
}

export function sendMessage(r: Omit<SendRequest, "sig">): string {
  return r.source + r.dest + r.amount + r.requestId + String(r.expiry);
}

export async function buildSend(privateKey: CryptoKey, source: string, dest: string, wei: bigint): Promise<SendRequest> {
  if (wei <= 0n) throw new Error("amount must be positive");
  const unsigned = {
    source,
    dest,
    amount: wei.toString(), // decimal, no leading zeros: the only form the server accepts
    requestId: crypto.randomUUID(),
    expiry: Math.floor(Date.now() / 1000) + 120,
  };
  return { ...unsigned, sig: await signHex(privateKey, sendMessage(unsigned)) };
}

const req = await buildSend(pair.privateKey, source, dest, parseEther("0.01"));
const { txnid } = await call<{ txnid: string }>(token, "POST", "/api/v1/wallets/send", req);
```

Build the request when the user confirms, and send it at once: it expires two minutes later.
`txnid` is the transaction hash. The transaction has been broadcast, not mined; balances
change once it's in a block.

Things that break the signature (401):
- changing an address's letter case, trimming or reformatting anything after signing;
- sending `amount` as a number, or any form other than `wei.toString()`;
- `expiry` in milliseconds, or as a string;
- a local key that isn't the registered one.

`crypto.randomUUID()` only exists in a secure context (https, or http on localhost).


## Errors

| Status | Meaning | What the app should do |
|---|---|---|
| 400 | Malformed request: bad field, bad amount format, expired or too-far `expiry`, bad `requestId`, `sig4reg` that doesn't verify, email not matching the token | A client bug, or the user waited too long: rebuild the request. Log the text |
| 401 | Send signature doesn't verify | Don't retry. Check that the local key is the registered one (see above) and how the message was built |
| 403 | A token problem, **or** not yours (address, wallet, other user's email), **or** a key replacement without a valid `sig4rotate` | Check the token with `GET /authping`. If that gets a 403, log in again; otherwise it's a permission problem |
| 404 | `GET /users/{email}`: no key registered | Start registration |
| 409 | Send: `requestId` already used. Key registration: the key changed meanwhile | Send: see retries. Keyreg: reload the user's key and start again |
| 413 / 415 | Body over 1 MB / wrong `Content-Type` | Client bug |
| 500 | Server error; the text is generic | Show a generic error |
| 502 | The Ethereum node couldn't be reached, or rejected the transaction (e.g. insufficient funds for amount plus gas) | Show the text. Don't resend the same body |

**Retrying a send.** The server records the `requestId` once the signature checks out, and
before it broadcasts. So:
- **No response at all** (network error, timeout): resending the **same** body is safe.
  If the first attempt never arrived, this one goes through. If it did arrive, you get a 409,
  and the transfer may have happened; check the balances. The server can't yet tell you the
  first attempt's `txnid`.
- **502:** that `requestId` is used up, and the transaction most likely wasn't sent.
  Retrying needs a new request (new `requestId`, `expiry` and signature), so treat it as a
  new user confirmation.
- **409:** don't retry with a new id automatically, or a transfer could happen twice.


## Test against the vectors first

`test-vectors.json` has two published test keys (`key1`, `key2`) and request bodies whose
signatures your code must reproduce exactly. PKCS #1 v1.5 signatures are deterministic, so
the same key and message always give the same bytes. A Vitest example (use the `node`
environment, since jsdom has no `crypto.subtle`; importing JSON needs `resolveJsonModule`):

```ts
// @vitest-environment node
import { describe, expect, it } from "vitest";
import vectors from "../docs/delsign-api/test-vectors.json";
import { fromHex, sendMessage, signHex } from "./delsignCrypto";

type KeyName = keyof typeof vectors.keys;

const importKey = (name: KeyName) =>
  crypto.subtle.importKey(
    "pkcs8",
    fromHex(vectors.keys[name].privateKeyPkcs8Hex),
    { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
    false,
    ["sign"],
  );

describe("signing matches the delsign test vectors", () => {
  for (const v of vectors.vectors) {
    for (const s of v.signatures) {
      it(`${v.name}: ${s.field}`, async () => {
        const body = v.body as Record<string, unknown>;
        expect(await signHex(await importKey(s.key as KeyName), s.message)).toBe(s.signature);
        expect(body[s.field]).toBe(s.signature);
        if (s.field === "sig") expect(sendMessage(body as any)).toBe(s.message);
        if (s.field === "sig4rotate") expect(s.message).toBe(String(body.email) + String(body.pubkey));
      });
    }
  }
});
```

The vectors' send `expiry` is fixed, so a live server rejects them; they test only signing.
**Never use the test keys for anything else.**


## Changes from the earlier API

For updating an existing client. All of these are hard cutovers; the old forms are rejected.

- Tokens need an `exp` claim.
- Sends add `requestId` and `expiry`, and the signature covers
  `source + dest + amount + requestId + expiry` (it was `source + dest + amount`).
- Wei amounts are strings in sends and balances (they were JSON numbers).
- Replacing a registered key needs `sig4rotate`.
- `GET /api/v1/users/{email}` needs a token, and works only for your own email.
- Request bodies with unknown fields get a 400.
- Ownership failures are 403, node failures are 502, and lists are `[]` rather than `null`.
