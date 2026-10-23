# Baseline: before the true-up

Snapshot taken for **T0.1** in [01-true-up-plan.md](../01-true-up-plan.md), before any
changes. **T5.3** compares the final state against it.

| File | Contents |
|---|---|
| `environment.txt` | Capture time, commit, Go toolchain, govulncheck version |
| `go-list-m-u.txt` | `go list -m -u all`: every module in the build, with the available update in brackets |
| `govulncheck.txt` | Full `govulncheck ./...` text report, with call traces |
| `vulnsummary.py` | Turns `govulncheck -format json` output into the table below |

## Headline numbers

- **Dependencies:** 166 modules in the build graph; **136 have updates available.**
  Direct dependencies: go-ethereum v1.13.2 → v1.17.10, gorilla/mux v1.8.0 → v1.8.1,
  go-sqlite3 v1.14.17 → v1.14.52. golang-jwt/jwt v3, gddo and negroni v1 show no update,
  because their maintained successors use new module paths or don't exist.
- **Vulnerabilities:** run with Go 1.26.5 and govulncheck v1.9.0. "Called" means our code
  reaches the vulnerable function; "imported" means we import the package but don't call
  the vulnerable function; "required" means the module is in `go.mod` but the package isn't imported.

| Reachability | Count |
|---|---|
| called | 24 |
| imported | 7 |
| required | 27 |

| Reachability | Module | Fixed in | ID | Summary |
|---|---|---|---|---|
| called | github.com/consensys/gnark-crypto | v0.12.0 | GO-2025-4027 | Gnark-crypto doesn't range check input values during ECDSA and EdDSA deserialization in github.com/consensys/gnark-crypto |
| called | github.com/ethereum/go-ethereum | v1.13.15 | GO-2024-2819 | Denial of Service in github.com/ethereum/go-ethereum |
| called | github.com/ethereum/go-ethereum | v1.16.8 | GO-2026-4314 | High CPU usage leading to DoS via malicious p2p message in github.com/ethereum/go-ethereum |
| called | github.com/ethereum/go-ethereum | v1.16.8 | GO-2026-4315 | DoS via malicious p2p message affecting a vulnerable node in github.com/ethereum/go-ethereum |
| called | github.com/ethereum/go-ethereum | v1.16.9 | GO-2026-4507 | Go Ethereum affected by crash via malicious p2p message in github.com/ethereum/go-ethereum |
| called | github.com/ethereum/go-ethereum | v1.17.0 | GO-2026-4508 | Go Ethereum affected by DoS via malicious p2p message in github.com/ethereum/go-ethereum |
| called | github.com/golang-jwt/jwt | none | GO-2025-3553 | Excessive memory allocation during header parsing in github.com/golang-jwt/jwt |
| called | github.com/gorilla/websocket | v1.5.3 | GO-2026-6278 | Gorilla WebSocket Uses Cryptographically Weak PRNG for WebSocket Mask Key in github.com/gorilla/websocket |
| called | stdlib | v1.26.6 | GO-2026-5026 | Invoking failure to reject ASCII-only Punycode-encoded labels in golang.org/x/net/idna |
| called | stdlib | v1.26.6 | GO-2026-5972 | Enforce maximum recursion depth in encoding/asn1 |
| called | stdlib | v1.26.6 | GO-2026-6089 | Apply ReadHeaderTimeout when doing unencrypted HTTP/2 check in net/http |
| called | stdlib | v1.26.6 | GO-2026-6090 | Limit handshake messages we are willing to accept post-handshake in crypto/tls |
| called | stdlib | v1.26.6 | GO-2026-6218 | Avoid quadratic complexity in resolvePath in net/url |
| called | stdlib | v1.26.9 | GO-2026-6603 | HTTP/2 server memory exhaustion due to Trailer headers in net/http |
| called | stdlib | v1.26.9 | GO-2026-6605 | HTTP/1 client connection desynchronization after CONNECT rejection in net/http |
| called | stdlib | v1.26.9 | GO-2026-6607 | Reject malformed ECH outer extension references in crypto/tls |
| called | stdlib | v1.26.9 | GO-2026-6608 | Memory limit bypass when parsing MIME headers in net/textproto, mime/multipart |
| called | stdlib | v1.26.9 | GO-2026-6609 | Lack of limit on size of parsed Range headers in net/http |
| called | stdlib | v1.26.9 | GO-2026-6610 | HTTP/2 transport accepts malformed framing-related headers in net/http |
| called | stdlib | v1.26.9 | GO-2026-6611 | Excessive CPU consumption from repeated initial window changes in net/http |
| called | stdlib | v1.26.9 | GO-2026-6612 | Double flow control refund on HTTP/2 server streams in net/http |
| called | stdlib | v1.26.9 | GO-2026-6613 | HTTP/1 server connection desynchronization after 2xx CONNECT response in net/http |
| called | stdlib | v1.26.9 | GO-2026-6617 | HTTP/2 server crash due to HPACK encoder race in net/http |
| called | stdlib | v1.26.10 | GO-2026-6809 | Distrust compromised intermediate certificates in crypto/x509 |
| imported | github.com/consensys/gnark-crypto | v0.12.1 | GO-2023-2101 | Incorrect exponentiation results in github.com/consensys/gnark-crypto |
| imported | github.com/consensys/gnark-crypto | v0.18.1 | GO-2025-4087 | Unchecked memory allocation during vector deserialization in github.com/consensys/gnark-crypto |
| imported | stdlib | v1.26.6 | GO-2026-5942 | Parsing an invalid SVCB or HTTPS RR can panic in golang.org/x/net/dns/dnsmessage |
| imported | stdlib | v1.26.6 | GO-2026-6091 | Fix Javascript regexp context tracking in html/template |
| imported | stdlib | v1.26.9 | GO-2026-6599 | Reset context tracking on consecutive template expressions in html/template |
| imported | stdlib | v1.26.9 | GO-2026-6600 | Recognize yield as regexp preceder keyword in html/template |
| imported | stdlib | v1.26.9 | GO-2026-6604 | Root.Mkdir(All) can follow junctions out of the root on Windows in os |
| required | github.com/consensys/gnark-crypto | v0.12.0 | GO-2023-2096 | Signature malleability in github.com/consensys/gnark-crypto |
| required | github.com/ethereum/go-ethereum | v1.16.9 | GO-2026-4511 | Go Ethereum Improperly Validates the ECIES Public Key in RLPx Handshake in github.com/ethereum/go-ethereum |
| required | golang.org/x/crypto | v0.17.0 | GO-2023-2402 | Man-in-the-middle attacker can compromise integrity of secure channel in golang.org/x/crypto |
| required | golang.org/x/crypto | v0.31.0 | GO-2024-3321 | Misuse of connection.serverAuthenticate may cause authorization bypass in golang.org/x/crypto |
| required | golang.org/x/crypto | v0.35.0 | GO-2025-3487 | Potential denial of service in golang.org/x/crypto |
| required | golang.org/x/crypto | v0.43.0 | GO-2025-4116 | Potential denial of service in golang.org/x/crypto/ssh/agent |
| required | golang.org/x/crypto | v0.45.0 | GO-2025-4134 | Unbounded memory consumption in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.45.0 | GO-2025-4135 | Malformed constraint may cause denial of service in golang.org/x/crypto/ssh/agent |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5005 | Invoking key constraints not enforced in golang.org/x/crypto/ssh/agent |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5006 | Invoking agent constraints dropped when forwarding keys in golang.org/x/crypto/ssh/agent |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5013 | Invoking byte arithmetic causes underflow and panic in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5014 | Invoking bypass of certificate restrictions in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5015 | Invoking server panic during CheckHostKey/Authenticate in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5016 | Invoking memory leak when rejecting channels can lead to DoS in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5017 | Invoking client can cause server deadlock on unexpected responses in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5018 | Invoking pathological RSA/DSA parameters may cause DoS in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5019 | Invoking bypass of FIDO/U2F security keys physical interaction in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5020 | Invoking infinite loop on large channel writes in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5021 | Invoking auth bypass via unenforced @revoked status in golang.org/x/crypto/ssh/knownhosts |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5023 | Invoking VerifiedPublicKeyCallback permissions skip enforcement in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.52.0 | GO-2026-5033 | Invoking pathological inputs can lead to client panic in golang.org/x/crypto/ssh/agent |
| required | golang.org/x/crypto | none | GO-2026-5932 | The golang.org/x/crypto/openpgp package is unmaintained, unsafe by design, and has known security issues |
| required | golang.org/x/crypto | v0.55.0 | GO-2026-6303 | Source-address critical option not enforced for non-public-key auth callbacks in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.56.0 | GO-2026-6354 | Prevent DoS on deadlocked undecided channel in golang.org/x/crypto/ssh |
| required | golang.org/x/crypto | v0.56.0 | GO-2026-6355 | Prevent DoS on deadlocked established channel in golang.org/x/crypto/ssh |
| required | golang.org/x/sys | v0.44.0 | GO-2026-5024 | Invoking integer overflow in NewNTUnicodeString in golang.org/x/sys/windows |
| required | stdlib | v1.26.6 | GO-2026-6088 | Add recursion depth guard during decode in encoding/xml |

## Reproduce

```bash
go list -m -u all
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -format json ./... > /tmp/vuln.json
python3 -I plan/baseline/vulnsummary.py /tmp/vuln.json
```

govulncheck exits with status 3 when it finds vulnerabilities. Under `go run`, that shows up as
`exit status 3` on the last line of `govulncheck.txt`. It is not an error.

The raw JSON report (about 2.8 MB) isn't kept; the table above is generated from it.
