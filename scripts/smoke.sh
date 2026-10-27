#!/usr/bin/env bash
#
# End-to-end smoke test against a local dev chain.
#
# Builds and starts the server on a scratch database, then runs:
#   register key -> create wallet -> create two addresses -> fund the first
#   from the node -> send ETH to the second through the API -> check the balance.
#
# Needs a running dev node with unlocked, funded accounts (Anvil, Hardhat, ...):
#   anvil &
#   scripts/smoke.sh
#
# Funding uses eth_sendTransaction from the node's first account, so the script
# never touches the database directly. The chain ID is read from the node.
#
# Environment:
#   SMOKE_RPC_URL  node JSON-RPC endpoint   (default http://localhost:8545)
#   SMOKE_PORT     port for the test server (default 3099)
#   SMOKE_KEEP=1   keep the temp dir (DB, keys, server log) for inspection

set -euo pipefail

RPC_URL=${SMOKE_RPC_URL:-http://localhost:8545}
PORT=${SMOKE_PORT:-3099}
API=http://localhost:$PORT/api/v1
EMAIL=smoke@example.com
FUND_WEI=1000000000000000000 # 1 ETH
SEND_WEI=100000000000000000  # 0.1 ETH

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d)
SERVER_PID=

cleanup() {
	if [[ -n $SERVER_PID ]]; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	if [[ ${SMOKE_KEEP:-} == 1 ]]; then
		echo "kept $WORK"
	else
		rm -rf "$WORK"
	fi
}
trap cleanup EXIT

step() { echo "==> $*"; }

fail() {
	echo "FAIL: $*" >&2
	if [[ -f $WORK/server.log ]]; then
		echo "--- server log (last 30 lines) ---" >&2
		tail -n 30 "$WORK/server.log" >&2
	fi
	exit 1
}

for tool in go curl jq openssl od; do
	command -v "$tool" >/dev/null || fail "$tool not found"
done

hexdump() { od -An -v -tx1 | tr -d ' \n'; }
b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }

# rpc METHOD PARAMS_JSON -> prints .result
rpc() {
	local resp
	resp=$(curl -s -X POST -H 'Content-Type: application/json' \
		--data "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"$1\",\"params\":$2}" "$RPC_URL") ||
		fail "$1: node not reachable at $RPC_URL"
	if [[ $(jq 'has("error")' <<<"$resp") == true ]]; then
		fail "$1: $(jq -c .error <<<"$resp")"
	fi
	jq -r .result <<<"$resp"
}

# api METHOD PATH [BODY] -> prints the response body; fails on a non-2xx status
api() {
	local args=(-sS -X "$1" -H "Authorization: Bearer $TOKEN" -o "$WORK/resp" -w '%{http_code}')
	if [[ $# -ge 3 ]]; then
		args+=(-H 'Content-Type: application/json' --data "$3")
	fi
	local status
	status=$(curl "${args[@]}" "$API$2") || fail "$1 $2: request failed"
	[[ $status == 2* ]] || fail "$1 $2: HTTP $status: $(cat "$WORK/resp")"
	cat "$WORK/resp"
}

wait_receipt() {
	local receipt
	for _ in $(seq 1 30); do
		receipt=$(rpc eth_getTransactionReceipt "[\"$1\"]")
		if [[ $receipt != null ]]; then
			[[ $(jq -r .status <<<"$receipt") == 0x1 ]] || fail "transaction $1 reverted"
			return
		fi
		sleep 1
	done
	fail "no receipt for $1 after 30s"
}

step "check node at $RPC_URL"
CHAIN_ID=$(rpc eth_chainId '[]')
CHAIN_ID=$(printf '%d' "$CHAIN_ID")
FUNDER=$(rpc eth_accounts '[]' | jq -r '.[0] // empty')
[[ -n $FUNDER ]] || fail "node has no unlocked accounts to fund from"
echo "    chain id $CHAIN_ID, funding from $FUNDER"

step "build and start server on :$PORT"
! curl -s -o /dev/null "http://localhost:$PORT/" || fail "port $PORT is already in use (set SMOKE_PORT)"
(cd "$ROOT" && go build -o "$WORK/delsignbackend" .)
JWT_SECRET=$(openssl rand -hex 32)
DELSIGN_JWT_SECRET=$JWT_SECRET \
	DELSIGN_PORT=$PORT \
	DELSIGN_DB_PATH=$WORK/smoke.db \
	DELSIGN_RPC_URL=$RPC_URL \
	DELSIGN_CHAIN_ID=$CHAIN_ID \
	"$WORK/delsignbackend" >"$WORK/server.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 50); do
	curl -s -o /dev/null "$API/authping" && break
	kill -0 "$SERVER_PID" 2>/dev/null || fail "server exited during startup"
	sleep 0.1
done
curl -s -o /dev/null "$API/authping" || fail "server did not start on :$PORT"

# HS256 token with an email claim, signed with the server's secret
header=$(printf '{"alg":"HS256","typ":"JWT"}' | b64url)
claims=$(printf '{"email":"%s","exp":%d}' "$EMAIL" $(($(date +%s) + 600)) | b64url)
sig=$(printf '%s.%s' "$header" "$claims" | openssl dgst -sha256 -hmac "$JWT_SECRET" -binary | b64url)
TOKEN=$header.$claims.$sig

# RSA key the user signs requests with (PKCS #1 v1.5, SHA-256, hex)
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$WORK/user.pem" 2>/dev/null
rsa_sign() { printf '%s' "$1" | openssl dgst -sha256 -sign "$WORK/user.pem" | hexdump; }

step "register key for $EMAIL"
pubkey=$(openssl pkey -in "$WORK/user.pem" -pubout -outform DER | hexdump)
api POST /keyreg "$(jq -nc --arg e "$EMAIL" --arg p "$pubkey" --arg s "$(rsa_sign "$EMAIL")" \
	'{email: $e, pubkey: $p, sig4reg: $s}')" >/dev/null

step "create wallet"
WALLET_ID=$(api POST /wallets | jq -r .id)
echo "    wallet $WALLET_ID"

step "create source and destination addresses"
SOURCE=$(api POST "/wallets/$WALLET_ID/addresses" | jq -r .eoa)
DEST=$(api POST "/wallets/$WALLET_ID/addresses" | jq -r .eoa)
echo "    source $SOURCE"
echo "    dest   $DEST"

step "fund source with $FUND_WEI wei from $FUNDER"
fund_tx=$(rpc eth_sendTransaction "$(jq -nc --arg f "$FUNDER" --arg t "$SOURCE" \
	--arg v "$(printf '0x%x' "$FUND_WEI")" '[{from: $f, to: $t, value: $v}]')")
wait_receipt "$fund_tx"
balance=$(api GET "/wallets/balance/$SOURCE" | jq -r .amount)
[[ $balance == "$FUND_WEI" ]] || fail "source balance $balance, want $FUND_WEI"

step "send $SEND_WEI wei from source to dest via the API"
send_sig=$(rsa_sign "$SOURCE$DEST$SEND_WEI")
TXNID=$(api POST /wallets/send "$(jq -nc --arg s "$SOURCE" --arg d "$DEST" \
	--argjson a "$SEND_WEI" --arg sig "$send_sig" '{source: $s, dest: $d, amount: $a, sig: $sig}')" |
	jq -r .txnid)
echo "    txn $TXNID"
wait_receipt "$TXNID"

step "check dest balance"
balance=$(api GET "/wallets/balance/$DEST" | jq -r .amount)
[[ $balance == "$SEND_WEI" ]] || fail "dest balance $balance, want $SEND_WEI"

echo "PASS"
