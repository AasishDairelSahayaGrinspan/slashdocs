#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="${AIDOCS_VERIFY_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)-$$}"
TMP_ROOT="${AIDOCS_VERIFY_TMPDIR:-$(mktemp -d "${TMPDIR:-/tmp}/aidocs-production-verify.XXXXXX")}"
mkdir -p "$TMP_ROOT"
# Keep diagnostics stable when macOS exposes the same temporary directory via
# both /var and /private/var.
TMP_ROOT="$(CDPATH= cd -- "$TMP_ROOT" && pwd -P)"
KEEP="${AIDOCS_VERIFY_KEEP:-0}"
VERIFY_IMAGE="${AIDOCS_VERIFY_IMAGE:-0}"

POSTGRES_IMAGE="${AIDOCS_VERIFY_POSTGRES_IMAGE:-postgres@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777}"
MINIO_IMAGE="${AIDOCS_VERIFY_MINIO_IMAGE:-minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e}"
MC_IMAGE="${AIDOCS_VERIFY_MC_IMAGE:-minio/mc@sha256:a7fe349ef4bd8521fb8497f55c6042871b2ae640607cf99d9bede5e9bdf11727}"
DEX_IMAGE="${AIDOCS_VERIFY_DEX_IMAGE:-dexidp/dex@sha256:bc7cfce7c17f52864e2bb2a4dc1d2f86a41e3019f6d42e81d92a301fad0c8a1d}"

NETWORK_NAME="aidocs-verify-${RUN_ID}"
POSTGRES_NAME="aidocs-verify-postgres-${RUN_ID}"
MINIO_NAME="aidocs-verify-minio-${RUN_ID}"
DEX_NAME="aidocs-verify-dex-${RUN_ID}"
IMAGE_NAME="aidocs-verify-image-${RUN_ID}"
IMAGE_TAG="aidocs-production-verification:${RUN_ID}"
POSTGRES_USER="aidocs"
POSTGRES_PASSWORD="aidocs-production-verification"
POSTGRES_DATABASE="aidocs"
MINIO_USER="aidocsverify"
MINIO_PASSWORD="aidocs-verify-secret-key"
BLOB_BUCKET="aidocs-production-verification"
OWNER_TOKEN="aidocs_verify_owner_token_${RUN_ID}"
REVIEWER_TOKEN="aidocs_verify_reviewer_token_${RUN_ID}"
SLACK_SECRET="aidocs-slack-verification-secret"
DEVREV_SECRET="aidocs-devrev-verification-key"
OIDC_EMAIL="verification-user@example.com"
OIDC_PASSWORD="password"
OIDC_PASSWORD_HASH='$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W'
SERVER_PID=""
DEVREV_FIXTURE_PID=""
SERVER_BIN="$TMP_ROOT/aidocs-server"
SERVER_LOG="$TMP_ROOT/aidocs-server.log"
DEVREV_FIXTURE_LOG="$TMP_ROOT/devrev-fixture.log"
DEX_CONFIG="$TMP_ROOT/dex-config.yaml"
DEVREV_PAT="devrev-pat-${RUN_ID}-plaintext-sentinel"
CONNECTION_ENCRYPTION_KEY="MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
PERSONAL_CONNECTIONS_ENABLED=0

say() {
	printf '\n==> %s\n' "$*"
}

fail() {
	printf '\nproduction verification: FAIL: %s\n' "$*" >&2
	if [[ -f "$SERVER_LOG" ]]; then
		printf '%s\n' '--- aidocs server log (last 120 lines) ---' >&2
		tail -n 120 "$SERVER_LOG" >&2 || true
	fi
	if docker inspect "$IMAGE_NAME" >/dev/null 2>&1; then
		printf '%s\n' '--- aidocs image log (last 120 lines) ---' >&2
		docker logs --tail 120 "$IMAGE_NAME" >&2 || true
	fi
	if docker inspect "$DEX_NAME" >/dev/null 2>&1; then
		printf '%s\n' '--- Dex log (last 120 lines) ---' >&2
		docker logs --tail 120 "$DEX_NAME" >&2 || true
	fi
	if [[ -f "$DEVREV_FIXTURE_LOG" ]]; then
		printf '%s\n' '--- fake DevRev log (last 120 lines) ---' >&2
		tail -n 120 "$DEVREV_FIXTURE_LOG" >&2 || true
	fi
	exit 1
}

require_command() {
	command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

stop_server() {
	if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
	SERVER_PID=""
}

stop_devrev_fixture() {
	if [[ -n "$DEVREV_FIXTURE_PID" ]] && kill -0 "$DEVREV_FIXTURE_PID" 2>/dev/null; then
		kill "$DEVREV_FIXTURE_PID" 2>/dev/null || true
		wait "$DEVREV_FIXTURE_PID" 2>/dev/null || true
	fi
	DEVREV_FIXTURE_PID=""
}

cleanup() {
	local status=$?
	stop_server
	stop_devrev_fixture
	if [[ "$KEEP" == "1" ]]; then
		printf 'production verification: keeping resources for inspection\n' >&2
		printf '  temp: %s\n  postgres: %s\n  minio: %s\n  dex: %s\n  fake DevRev log: %s\n  image container: %s\n  image: %s\n  network: %s\n' "$TMP_ROOT" "$POSTGRES_NAME" "$MINIO_NAME" "$DEX_NAME" "$DEVREV_FIXTURE_LOG" "$IMAGE_NAME" "$IMAGE_TAG" "$NETWORK_NAME" >&2
	else
		docker unpause "$MINIO_NAME" >/dev/null 2>&1 || true
		docker rm -f "$IMAGE_NAME" "$DEX_NAME" "$MINIO_NAME" "$POSTGRES_NAME" >/dev/null 2>&1 || true
		if [[ "$VERIFY_IMAGE" == "1" ]]; then
			docker image rm -f "$IMAGE_TAG" >/dev/null 2>&1 || true
		fi
		docker network rm "$NETWORK_NAME" >/dev/null 2>&1 || true
		rm -rf "$TMP_ROOT"
	fi
	exit "$status"
}
trap cleanup EXIT INT TERM

free_port() {
	python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()'
}

wait_for() {
	local label="$1"
	shift
	local attempt
	for attempt in $(seq 1 90); do
		if "$@" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	fail "timed out waiting for $label"
}

expect_http() {
	local expected="$1"
	local output="$2"
	shift 2
	local actual
	actual="$(curl --silent --show-error --output "$output" --write-out '%{http_code}' "$@" || true)"
	if [[ "$actual" != "$expected" ]]; then
		printf '%s\n' "--- unexpected response body ($actual; expected $expected) ---" >&2
		cat "$output" >&2 || true
		fail "unexpected HTTP status"
	fi
}

assert_json() {
	local file="$1"
	shift
	jq -e "$@" "$file" >/dev/null || {
		printf '%s' '--- JSON assertion failed:' >&2
		printf ' %q' "$@" >&2
		printf '%s\n' ' ---' >&2
		cat "$file" >&2
		fail "JSON assertion failed"
	}
}

owner_get() {
	local output="$1"
	local path="$2"
	expect_http 200 "$output" -H "Authorization: Bearer $OWNER_TOKEN" "$BASE_URL$path"
}

wait_for_sync_run() {
	local source_id="$1"
	local delivery_id="$2"
	local output="$3"
	local attempt
	for attempt in $(seq 1 90); do
		curl --silent --show-error -H "Authorization: Bearer $OWNER_TOKEN" "$BASE_URL/v1/repositories/$REPOSITORY_ID/sync-runs" >"$output"
		if jq -e --arg source "$source_id" --arg delivery "$delivery_id" '.items[] | select(.source_id == $source and .delivery_id == $delivery and (.status == "completed" or .status == "failed"))' "$output" >/dev/null; then
			return 0
		fi
		sleep 1
	done
	fail "timed out waiting for source $source_id delivery $delivery_id"
}

slack_signature() {
	local timestamp="$1"
	local body="$2"
	local digest
	digest="$(printf 'v0:%s:%s' "$timestamp" "$body" | openssl dgst -sha256 -hmac "$SLACK_SECRET" | awk '{print $NF}')"
	printf 'v0=%s' "$digest"
}

devrev_signature() {
	local body="$1"
	printf '%s' "$body" | openssl dgst -sha256 -hmac "$DEVREV_SECRET" | awk '{print $NF}'
}

minio_object_count() {
	docker run --rm --network "$NETWORK_NAME" --entrypoint /bin/sh \
		--env VERIFY_ENDPOINT="http://$MINIO_NAME:9000" \
		--env VERIFY_ACCESS_KEY="$MINIO_USER" \
		--env VERIFY_SECRET_KEY="$MINIO_PASSWORD" \
		--env VERIFY_BUCKET="$BLOB_BUCKET" \
		"$MC_IMAGE" -c '
			set -eu
			mc alias set verify "$VERIFY_ENDPOINT" "$VERIFY_ACCESS_KEY" "$VERIFY_SECRET_KEY" >/dev/null
			mc find "verify/$VERIFY_BUCKET" >/tmp/aidocs-objects
			wc -l </tmp/aidocs-objects | tr -d " "
		'
}

database_import_shape() {
	psql "$DATABASE_URL" -Atqc "
		SELECT concat_ws('|',
			(SELECT count(*) FROM repositories),
			(SELECT count(*) FROM folders),
			(SELECT count(*) FROM documents),
			(SELECT count(*) FROM versions)
		)"
}

minutes_ago_iso() {
	local minutes="$1"
	if date -u -v-"${minutes}"M +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
		date -u -v-"${minutes}"M +%Y-%m-%dT%H:%M:%SZ
	else
		date -u -d "${minutes} minutes ago" +%Y-%m-%dT%H:%M:%SZ
	fi
}

write_dex_config() {
	cat >"$DEX_CONFIG" <<EOF
issuer: $OIDC_ISSUER
storage:
  type: memory
web:
  http: 0.0.0.0:5556
oauth2:
  skipApprovalScreen: true
staticClients:
  - id: aidocs-production-verification
    secret: aidocs-production-verification-oidc-secret
    name: aidocs production verification
    redirectURIs:
      - $BASE_URL/v1/auth/oidc/callback
enablePasswordDB: true
staticPasswords:
  - email: $OIDC_EMAIL
    hash: "$OIDC_PASSWORD_HASH"
    username: verification
    userID: 08a8684b-db88-4b73-90a9-3cd1661f5466
EOF
}

start_devrev_fixture() {
	: >"$DEVREV_FIXTURE_LOG"
	DEVREV_FIXTURE_PAT="$DEVREV_PAT" python3 -u - "$DEVREV_FIXTURE_PORT" >"$DEVREV_FIXTURE_LOG" 2>&1 <<'PY' &
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

port = int(sys.argv[1])
expected_token = os.environ["DEVREV_FIXTURE_PAT"]


class Handler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        # Keep diagnostics useful without ever logging Authorization headers.
        sys.stderr.write("fake DevRev: %s %s\n" % (self.command, self.path))

    def send_json(self, status, payload):
        raw = json.dumps(payload, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def read_json(self):
        length = int(self.headers.get("Content-Length", "0"))
        return json.loads(self.rfile.read(length) or b"{}")

    def do_GET(self):
        if self.path == "/readyz":
            self.send_json(200, {"status": "ok"})
            return
        self.send_json(404, {"error": "not_found"})

    def do_POST(self):
        if self.headers.get("Authorization") != "Bearer " + expected_token:
            self.send_json(401, {"error": "unauthorized"})
            return
        try:
            payload = self.read_json()
        except (ValueError, json.JSONDecodeError):
            self.send_json(400, {"error": "invalid_json"})
            return
        if self.path == "/auth-tokens.info":
            if payload != {}:
                self.send_json(400, {"error": "unexpected_body"})
                return
            self.send_json(200, {
                "organization": {
                    "id": "don:identity:dvrv-us-1:devo/verify",
                    "display_id": "DEV-VERIFY",
                    "slug": "aidocs-production-verification",
                },
                "token": {"expiry": 4102444800},
                "user": {
                    "id": "don:identity:dvrv-us-1:devu/verify",
                    "display_name": "DevRev Verification User",
                    "full_name": "DevRev Verification User",
                    "email": "verification-user@example.com",
                },
            })
            return
        if self.path == "/works.get":
            if payload != {"id": "TKT-9001"}:
                self.send_json(400, {"error": "unexpected_work"})
                return
            self.send_json(200, {"work": {
                "id": "don:core:dvrv-us-1:devo/verify:ticket/9001",
                "display_id": "TKT-9001",
                "type": "ticket",
                "title": "Personal DevRev knowledge is stale",
                "body": "Refresh the settlement recovery steps from the linked ticket.",
                "state": "open",
                "modified_date": "2026-07-15T12:00:00Z",
                "web_url": "https://app.devrev.example/tickets/TKT-9001",
            }})
            return
        self.send_json(404, {"error": "not_found"})


HTTPServer(("127.0.0.1", port), Handler).serve_forever()
PY
	DEVREV_FIXTURE_PID=$!
	wait_for "fake DevRev provider" curl --fail --silent "$DEVREV_FIXTURE_URL/readyz"
}

start_server() {
	local connection_encryption_key=""
	local devrev_api_base_url=""
	if [[ "$PERSONAL_CONNECTIONS_ENABLED" == "1" ]]; then
		connection_encryption_key="$CONNECTION_ENCRYPTION_KEY"
		devrev_api_base_url="$DEVREV_FIXTURE_URL"
	fi
	: >"$SERVER_LOG"
	env \
		AIDOCS_CONNECTION_ENCRYPTION_KEY="$connection_encryption_key" \
		AIDOCS_DEVREV_API_BASE_URL="$devrev_api_base_url" \
		DATABASE_URL="$DATABASE_URL" \
		BLOB_BUCKET="$BLOB_BUCKET" \
		BLOB_REGION="us-east-1" \
		BLOB_ENDPOINT="$MINIO_ENDPOINT" \
		BLOB_FORCE_PATH_STYLE="true" \
		BLOB_ACCESS_KEY_ID="$MINIO_USER" \
		BLOB_SECRET_ACCESS_KEY="$MINIO_PASSWORD" \
		AIDOCS_PORT="$APP_PORT" \
		AIDOCS_METRICS_PORT="$METRICS_PORT" \
		AIDOCS_APP_ORIGIN="$BASE_URL" \
		AIDOCS_RENDER_ORIGIN="http://localhost:$APP_PORT" \
		AIDOCS_DEPLOYMENT="org" \
		AIDOCS_ORG_DOMAINS="example.com" \
		AIDOCS_ORG_NAME="Production Verification" \
		AIDOCS_MIGRATE="true" \
		AIDOCS_SOURCE_POLL_INTERVAL="0" \
		AIDOCS_OIDC_ISSUER_URL="$OIDC_ISSUER" \
		AIDOCS_OIDC_CLIENT_ID="aidocs-production-verification" \
		AIDOCS_OIDC_CLIENT_SECRET="aidocs-production-verification-oidc-secret" \
		AIDOCS_OIDC_PROVIDER_NAME="Dex verification" \
		SESSION_SECRET="aidocs-production-verification-session-secret" \
		AIDOCS_GITHUB_WEBHOOK_SECRET="aidocs-github-verification-secret" \
		AIDOCS_SLACK_SIGNING_SECRET="$SLACK_SECRET" \
		AIDOCS_DEVREV_WEBHOOK_SECRET="$DEVREV_SECRET" \
		"$SERVER_BIN" >"$SERVER_LOG" 2>&1 &
	SERVER_PID=$!
	wait_for "production server readiness" curl --fail --silent "$BASE_URL/readyz"
}

for command in docker curl jq psql openssl shasum go npm helm python3 od; do
	require_command "$command"
done
docker info >/dev/null 2>&1 || fail "Docker is installed but its daemon is unavailable"
if [[ "$VERIFY_IMAGE" == "1" ]]; then
	docker buildx version >/dev/null 2>&1 || fail "actual-image verification requires the Docker Buildx plugin"
fi

mkdir -p "$TMP_ROOT/import/handbook/runbooks"
APP_PORT="$(free_port)"
METRICS_PORT="$(free_port)"
OIDC_PORT="$(free_port)"
DEVREV_FIXTURE_PORT="$(free_port)"
BASE_URL="http://127.0.0.1:$APP_PORT"
OIDC_ISSUER="http://127.0.0.1:$OIDC_PORT/dex"
DEVREV_FIXTURE_URL="http://127.0.0.1:$DEVREV_FIXTURE_PORT"
write_dex_config
start_devrev_fixture

say "Build the exact production frontend and server command"
(
	cd "$ROOT_DIR/frontend"
	npm ci
	npm run build
)
(
	cd "$ROOT_DIR"
	bridge_source_hash="$(node frontend/scripts/bridge-source-hash.mjs)"
	[[ ${#bridge_source_hash} -eq 64 ]] || fail "invalid bridge source fingerprint"
	go build -trimpath \
		-ldflags "-X github.com/razorpay/aidocs/api/internal/server.BuildVersion=production-verification -X github.com/razorpay/aidocs/api/internal/server.BuildCommit=$RUN_ID -X github.com/razorpay/aidocs/api/internal/server.BuildBridgeSourceHash=$bridge_source_hash" \
		-o "$SERVER_BIN" ./api/cmd/aidocs-server
)

say "Render and lint the production Helm deployment"
HELM_ARGS=(
	--set-string image.tag=production-verification
	--set-string oidc.issuerURL=https://sso.example.com
	--set-string oidc.internalIssuerURL=http://dex:5556
	--set-string oidc.providerName='Verification OIDC'
	--set-string oidc.clientID=aidocs-production-verification
	--set-string oidc.clientSecret=aidocs-production-verification-oidc-secret
	--set-string session.secret=aidocs-production-verification-session-secret
	--set-string database.host=postgres.example.internal
	--set-string database.user=aidocs
	--set-string database.password=verification-password
	--set-string database.name=aidocs
	--set-string blob.bucket=aidocs-verification
	--set-string blob.accessKeyID=verification-access-key
	--set-string blob.secretAccessKey=verification-secret-key
	--set-string personalConnections.encryption.key=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
	--set-string personalConnections.github.clientID=github-oauth-verification-id
	--set-string personalConnections.github.clientSecret=github-oauth-verification-secret
	--set-string personalConnections.slack.clientID=slack-oauth-verification-id
	--set-string personalConnections.slack.clientSecret=slack-oauth-verification-secret
	--set-string personalConnections.googleDrive.clientID=google-drive-oauth-verification-id
	--set-string personalConnections.googleDrive.clientSecret=google-drive-oauth-verification-secret
	--set-string personalConnections.googlePicker.apiKey=google-picker-verification-key
	--set-string personalConnections.googlePicker.appID=123456789012
	--set-string personalConnections.devRev.apiBaseURL=https://api.devrev.example
	--set-string connectedKnowledge.pollInterval=30m
	--set-string connectedKnowledge.githubWebhookSecret=github-verification-secret
	--set-string connectedKnowledge.slackSigningSecret=slack-verification-secret
	--set-string connectedKnowledge.devRevWebhookSecret=devrev-verification-secret
)
helm lint "$ROOT_DIR/charts/aidocs" "${HELM_ARGS[@]}"
helm template aidocs "$ROOT_DIR/charts/aidocs" "${HELM_ARGS[@]}" >"$TMP_ROOT/helm-rendered.yaml"
for env_name in DATABASE_URL BLOB_BUCKET AIDOCS_OIDC_ISSUER_URL AIDOCS_OIDC_INTERNAL_ISSUER_URL AIDOCS_OIDC_PROVIDER_NAME AIDOCS_OIDC_CLIENT_ID AIDOCS_OIDC_CLIENT_SECRET AIDOCS_SOURCE_POLL_INTERVAL AIDOCS_CONNECTION_ENCRYPTION_KEY AIDOCS_GITHUB_OAUTH_CLIENT_ID AIDOCS_GITHUB_OAUTH_CLIENT_SECRET AIDOCS_SLACK_OAUTH_CLIENT_ID AIDOCS_SLACK_OAUTH_CLIENT_SECRET AIDOCS_GOOGLE_DRIVE_OAUTH_CLIENT_ID AIDOCS_GOOGLE_DRIVE_OAUTH_CLIENT_SECRET AIDOCS_GOOGLE_PICKER_API_KEY AIDOCS_GOOGLE_PICKER_APP_ID AIDOCS_DEVREV_API_BASE_URL AIDOCS_GITHUB_WEBHOOK_SECRET AIDOCS_SLACK_SIGNING_SECRET AIDOCS_DEVREV_WEBHOOK_SECRET; do
	grep -q "name: $env_name" "$TMP_ROOT/helm-rendered.yaml" || fail "Helm deployment does not wire $env_name"
done

say "Start isolated PostgreSQL, S3-compatible MinIO, and Dex OIDC"
docker network create "$NETWORK_NAME" >/dev/null
docker run --detach --name "$POSTGRES_NAME" --network "$NETWORK_NAME" \
	--publish 127.0.0.1::5432 \
	--env POSTGRES_USER="$POSTGRES_USER" \
	--env POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
	--env POSTGRES_DB="$POSTGRES_DATABASE" \
	"$POSTGRES_IMAGE" >/dev/null
docker run --detach --name "$MINIO_NAME" --network "$NETWORK_NAME" \
	--publish 127.0.0.1::9000 \
	--env MINIO_ROOT_USER="$MINIO_USER" \
	--env MINIO_ROOT_PASSWORD="$MINIO_PASSWORD" \
	"$MINIO_IMAGE" server /data --address :9000 --console-address :9001 >/dev/null
docker create --name "$DEX_NAME" --network "$NETWORK_NAME" \
	--publish "127.0.0.1:$OIDC_PORT:5556" \
	"$DEX_IMAGE" dex serve /etc/dex/config.yaml >/dev/null
docker cp "$DEX_CONFIG" "$DEX_NAME:/etc/dex/config.yaml"
docker start "$DEX_NAME" >/dev/null

wait_for "PostgreSQL" docker exec "$POSTGRES_NAME" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DATABASE"
POSTGRES_PORT="$(docker port "$POSTGRES_NAME" 5432/tcp | awk -F: 'NR == 1 {print $NF}')"
MINIO_PORT="$(docker port "$MINIO_NAME" 9000/tcp | awk -F: 'NR == 1 {print $NF}')"
DATABASE_URL="postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@127.0.0.1:$POSTGRES_PORT/$POSTGRES_DATABASE?sslmode=disable"
MINIO_ENDPOINT="http://127.0.0.1:$MINIO_PORT"
wait_for "MinIO" curl --fail --silent "$MINIO_ENDPOINT/minio/health/live"
wait_for "Dex OIDC provider" curl --fail --silent "$OIDC_ISSUER/healthz"
docker run --rm --network "$NETWORK_NAME" --entrypoint /bin/sh \
	--env VERIFY_ENDPOINT="http://$MINIO_NAME:9000" \
	--env VERIFY_ACCESS_KEY="$MINIO_USER" \
	--env VERIFY_SECRET_KEY="$MINIO_PASSWORD" \
	--env VERIFY_BUCKET="$BLOB_BUCKET" \
	"$MC_IMAGE" -c '
		mc alias set verify "$VERIFY_ENDPOINT" "$VERIFY_ACCESS_KEY" "$VERIFY_SECRET_KEY" >/dev/null
		mc mb --ignore-existing "verify/$VERIFY_BUCKET" >/dev/null
	'

say "Boot the production server, run migrations, and verify dependency readiness"
start_server
owner_get "$TMP_ROOT/ready.json" "/readyz"
assert_json "$TMP_ROOT/ready.json" '.status == "ok" and .checks.postgres == "ok" and .checks.blob == "ok"'
expect_http 200 "$TMP_ROOT/metrics.txt" "http://127.0.0.1:$METRICS_PORT/metrics"
grep -q 'aidocs_http_requests_total' "$TMP_ROOT/metrics.txt" || fail "Prometheus metrics were not exposed"
expect_http 200 "$TMP_ROOT/frontend.html" "$BASE_URL/app/repositories"
grep -q '<div id="root"></div>' "$TMP_ROOT/frontend.html" || fail "production frontend was not embedded"
expect_http 200 "$TMP_ROOT/config.json" "$BASE_URL/v1/config"
assert_json "$TMP_ROOT/config.json" '.provider_name == "Dex verification"'
expect_http 401 "$TMP_ROOT/unauthorized.json" "$BASE_URL/v1/repositories"

say "Complete a local OIDC authorization-code login through Dex"
OIDC_COOKIE_JAR="$TMP_ROOT/oidc-cookies.txt"
OIDC_LOGIN_URL="$(curl --silent --show-error --location \
	--cookie-jar "$OIDC_COOKIE_JAR" --cookie "$OIDC_COOKIE_JAR" \
	--output "$TMP_ROOT/oidc-login.html" --write-out '%{url_effective}' \
	"$BASE_URL/v1/auth/oidc/start?mode=web&redirect=/app/repositories")"
[[ "$OIDC_LOGIN_URL" == "$OIDC_ISSUER"/* ]] || fail "OIDC start reached $OIDC_LOGIN_URL instead of local Dex"
OIDC_LOGIN_ACTION="$(grep -o 'action="[^"]*"' "$TMP_ROOT/oidc-login.html" | head -n 1 | sed 's/^action="//; s/"$//; s/\&amp;/\&/g' || true)"
[[ -n "$OIDC_LOGIN_ACTION" ]] || fail "local Dex login form did not contain an action"
[[ "$OIDC_LOGIN_ACTION" == http* ]] || OIDC_LOGIN_ACTION="http://127.0.0.1:$OIDC_PORT$OIDC_LOGIN_ACTION"
OIDC_FINAL_URL="$(curl --silent --show-error --location \
	--cookie-jar "$OIDC_COOKIE_JAR" --cookie "$OIDC_COOKIE_JAR" \
	--output /dev/null --write-out '%{url_effective}' \
	--data-urlencode "login=$OIDC_EMAIL" --data-urlencode "password=$OIDC_PASSWORD" \
	"$OIDC_LOGIN_ACTION")"
[[ "$OIDC_FINAL_URL" == "$BASE_URL"/* ]] || fail "OIDC callback reached $OIDC_FINAL_URL instead of aidocs"
expect_http 200 "$TMP_ROOT/oidc-me.json" --cookie "$OIDC_COOKIE_JAR" "$BASE_URL/v1/me"
assert_json "$TMP_ROOT/oidc-me.json" '.principal.type == "user" and .user.email == "verification-user@example.com"'

OWNER_HASH="$(printf '%s' "$OWNER_TOKEN" | shasum -a 256 | awk '{print $1}')"
REVIEWER_HASH="$(printf '%s' "$REVIEWER_TOKEN" | shasum -a 256 | awk '{print $1}')"
psql "$DATABASE_URL" --set ON_ERROR_STOP=1 --set owner_hash="$OWNER_HASH" --set reviewer_hash="$REVIEWER_HASH" <<'SQL' >/dev/null
INSERT INTO users(id,email,name) VALUES
  ('verify_owner','owner@example.com','Verification Owner'),
  ('verify_reviewer','reviewer@example.com','Verification Reviewer')
ON CONFLICT (id) DO NOTHING;
INSERT INTO cli_credentials(id,user_id,name,token_hash) VALUES
  ('verify_owner_credential','verify_owner','production verification', :'owner_hash'),
  ('verify_reviewer_credential','verify_reviewer','production verification', :'reviewer_hash')
ON CONFLICT (id) DO NOTHING;
SQL
owner_get "$TMP_ROOT/me.json" "/v1/me"
assert_json "$TMP_ROOT/me.json" '.principal.id == "verify_owner" and .principal.type == "user" and .user.email == "owner@example.com"'

say "Import a local folder atomically and preserve its hierarchy"
printf '%s' '<!doctype html><html><body><h1>Payments overview</h1><p>Initial production content.</p></body></html>' >"$TMP_ROOT/import/handbook/overview.html"
printf '%s' '<!doctype html><html><body><h1>Incident response</h1><p>Escalate safely.</p></body></html>' >"$TMP_ROOT/import/handbook/runbooks/incident.html"
expect_http 201 "$TMP_ROOT/import-folder.json" \
	-X POST "$BASE_URL/v1/repositories/import" \
	-H "Authorization: Bearer $OWNER_TOKEN" \
	-F 'name=Production handbook' \
	-F 'slug=production-handbook' \
	-F 'description=Production parity verification repository' \
	-F 'scope=organization' \
	-F 'required_approvals=1' \
	-F 'protected=true' \
	-F 'import_kind=folder' \
	-F 'paths=overview.html' \
	-F "files=@$TMP_ROOT/import/handbook/overview.html;filename=overview.html;type=text/html" \
	-F 'paths=runbooks/incident.html' \
	-F "files=@$TMP_ROOT/import/handbook/runbooks/incident.html;filename=incident.html;type=text/html"
assert_json "$TMP_ROOT/import-folder.json" '.repository.id != "" and .repository.scope == "organization" and (.documents | length) == 2'
REPOSITORY_ID="$(jq -er '.repository.id' "$TMP_ROOT/import-folder.json")"
ROOT_FOLDER_ID="$(jq -er '.repository.root_folder_id' "$TMP_ROOT/import-folder.json")"
DOCUMENT_ID="$(jq -er '[.documents[] | select(.title | ascii_downcase | contains("overview"))][0].id // .documents[0].id' "$TMP_ROOT/import-folder.json")"
BASE_VERSION_ID="$(jq -er --arg id "$DOCUMENT_ID" '.documents[] | select(.id == $id) | .current_version_id' "$TMP_ROOT/import-folder.json")"
owner_get "$TMP_ROOT/root-folder.json" "/v1/folders?folder_id=$ROOT_FOLDER_ID"
assert_json "$TMP_ROOT/root-folder.json" '([.items[] | select(.type == "document")] | length >= 1) and ([.items[] | select(.type == "folder")] | length >= 1)'
IMPORTED_FOLDER_ID="$(jq -er '.items[] | select(.type == "folder") | .folder.id' "$TMP_ROOT/root-folder.json" | head -n 1)"
owner_get "$TMP_ROOT/imported-folder.json" "/v1/folders?folder_id=$IMPORTED_FOLDER_ID"
assert_json "$TMP_ROOT/imported-folder.json" '[.items[] | select(.type == "document")] | length >= 1'
owner_get "$TMP_ROOT/imported-html.html" "/v1/documents/$DOCUMENT_ID/versions/$BASE_VERSION_ID/html"
grep -q 'Initial production content' "$TMP_ROOT/imported-html.html" || fail "imported HTML was not readable from S3"

say "Import a loose local document into its own repository"
printf '%s' '<!doctype html><html><body><h1>Loose decision record</h1></body></html>' >"$TMP_ROOT/import/decision.html"
expect_http 201 "$TMP_ROOT/import-document.json" \
	-X POST "$BASE_URL/v1/repositories/import" \
	-H "Authorization: Bearer $OWNER_TOKEN" \
	-F 'name=Decision records' \
	-F 'slug=decision-records' \
	-F 'scope=personal' \
	-F 'import_kind=document' \
	-F 'paths=decision.html' \
	-F "files=@$TMP_ROOT/import/decision.html;filename=decision.html;type=text/html"
assert_json "$TMP_ROOT/import-document.json" '.repository.id != "" and .repository.scope == "personal" and (.documents | length) == 1'

say "Reject a conflicting multi-file import without metadata or S3 orphans"
printf '%s' '<!doctype html><html><body><p>rollback sentinel one</p></body></html>' >"$TMP_ROOT/import/rollback-one.html"
printf '%s' '<!doctype html><html><body><p>rollback sentinel two</p></body></html>' >"$TMP_ROOT/import/rollback-two.html"
ROLLBACK_DATABASE_BEFORE="$(database_import_shape)"
ROLLBACK_OBJECTS_BEFORE="$(minio_object_count)"
expect_http 409 "$TMP_ROOT/import-rollback.json" \
	-X POST "$BASE_URL/v1/repositories/import" \
	-H "Authorization: Bearer $OWNER_TOKEN" \
	-F 'name=Rollback sentinel repository' \
	-F 'slug=production-handbook' \
	-F 'scope=organization' \
	-F 'import_kind=folder' \
	-F 'paths=rollback-one.html' \
	-F "files=@$TMP_ROOT/import/rollback-one.html;filename=rollback-one.html;type=text/html" \
	-F 'paths=nested/rollback-two.html' \
	-F "files=@$TMP_ROOT/import/rollback-two.html;filename=rollback-two.html;type=text/html"
assert_json "$TMP_ROOT/import-rollback.json" '.error.code == "conflict"'
ROLLBACK_DATABASE_AFTER="$(database_import_shape)"
ROLLBACK_OBJECTS_AFTER="$(minio_object_count)"
[[ "$ROLLBACK_DATABASE_AFTER" == "$ROLLBACK_DATABASE_BEFORE" ]] || fail "failed import changed repository/folder/document/version counts ($ROLLBACK_DATABASE_BEFORE -> $ROLLBACK_DATABASE_AFTER)"
[[ "$ROLLBACK_OBJECTS_AFTER" == "$ROLLBACK_OBJECTS_BEFORE" ]] || fail "failed import leaked an S3 object ($ROLLBACK_OBJECTS_BEFORE -> $ROLLBACK_OBJECTS_AFTER)"
ROLLBACK_SENTINELS="$(psql "$DATABASE_URL" -Atqc "SELECT count(*) FROM folders WHERE title='Rollback sentinel repository'")"
[[ "$ROLLBACK_SENTINELS" == "0" ]] || fail "failed import left $ROLLBACK_SENTINELS rollback sentinel folders"

say "Prove org discovery, independent approval, protected merge, and S3-backed versions"
expect_http 200 "$TMP_ROOT/reviewer-repository.json" \
	-H "Authorization: Bearer $REVIEWER_TOKEN" "$BASE_URL/v1/repositories/$REPOSITORY_ID"
expect_http 201 "$TMP_ROOT/editor-grant.json" \
	-X POST "$BASE_URL/v1/folders/$ROOT_FOLDER_ID/grants" \
	-H "Authorization: Bearer $OWNER_TOKEN" \
	-H 'Content-Type: application/json' \
	--data '{"principal":{"type":"user","id":"verify_reviewer"},"role":"editor"}'

jq -n \
	--arg document "$DOCUMENT_ID" \
	--arg base "$BASE_VERSION_ID" \
	--arg html '<!doctype html><html><body><h1>Payments overview</h1><p>Approved production content.</p></body></html>' \
	'{title:"Verify governed document update",body:"Production verification",change_summary:"Change visible text",changes:[{operation:"update",document_id:$document,base_version_id:$base,content_html:$html}]}' \
	>"$TMP_ROOT/create-pr.json"
expect_http 201 "$TMP_ROOT/pr.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/pull-requests/change-set" \
	-H "Authorization: Bearer $OWNER_TOKEN" \
	-H 'Content-Type: application/json' \
	--data-binary "@$TMP_ROOT/create-pr.json"
PR_NUMBER="$(jq -er '.number' "$TMP_ROOT/pr.json")"
expect_http 403 "$TMP_ROOT/self-review.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/pull-requests/$PR_NUMBER/reviews" \
	-H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
	--data '{"state":"approved","body":"self approval must fail"}'
assert_json "$TMP_ROOT/self-review.json" '.error.code == "self_review"'
expect_http 409 "$TMP_ROOT/merge-blocked.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/pull-requests/$PR_NUMBER/merge" \
	-H "Authorization: Bearer $OWNER_TOKEN"
assert_json "$TMP_ROOT/merge-blocked.json" '.error.code == "approval_required"'
expect_http 200 "$TMP_ROOT/approved-pr.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/pull-requests/$PR_NUMBER/reviews" \
	-H "Authorization: Bearer $REVIEWER_TOKEN" -H 'Content-Type: application/json' \
	--data '{"state":"approved","body":"independently verified"}'
expect_http 200 "$TMP_ROOT/merged-pr.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/pull-requests/$PR_NUMBER/merge" \
	-H "Authorization: Bearer $OWNER_TOKEN"
assert_json "$TMP_ROOT/merged-pr.json" '.status == "merged"'
owner_get "$TMP_ROOT/document-after-merge.json" "/v1/documents/$DOCUMENT_ID"
CURRENT_VERSION_ID="$(jq -er '.current_version_id' "$TMP_ROOT/document-after-merge.json")"
[[ "$CURRENT_VERSION_ID" != "$BASE_VERSION_ID" ]] || fail "merge did not advance the document head"
owner_get "$TMP_ROOT/merged-html.html" "/v1/documents/$DOCUMENT_ID/versions/$CURRENT_VERSION_ID/html"
grep -q 'Approved production content' "$TMP_ROOT/merged-html.html" || fail "merged HTML was not stored"
owner_get "$TMP_ROOT/versions.json" "/v1/documents/$DOCUMENT_ID/versions"
assert_json "$TMP_ROOT/versions.json" '(.items | length) == 2'

say "Connect Slack threads and verify signed event -> durable run -> aidocs PR"
jq -n --arg document "$DOCUMENT_ID" '{provider:"slack",external_repository:"TVERIFY/CVERIFY",target_document_id:$document,trigger_mode:"event",direction:"into_aidocs"}' >"$TMP_ROOT/create-slack-source.json"
expect_http 201 "$TMP_ROOT/slack-source.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/sources" \
	-H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
	--data-binary "@$TMP_ROOT/create-slack-source.json"
SLACK_SOURCE_ID="$(jq -er '.id' "$TMP_ROOT/slack-source.json")"
SLACK_NOW="$(date -u +%s)"
SLACK_CHALLENGE_BODY='{"type":"url_verification","challenge":"aidocs-slack-challenge"}'
SLACK_CHALLENGE_SIGNATURE="$(slack_signature "$SLACK_NOW" "$SLACK_CHALLENGE_BODY")"
expect_http 200 "$TMP_ROOT/slack-challenge.json" \
	-X POST "$BASE_URL/v1/webhooks/slack" -H 'Content-Type: application/json' \
	-H "X-Slack-Request-Timestamp: $SLACK_NOW" -H "X-Slack-Signature: $SLACK_CHALLENGE_SIGNATURE" \
	--data-binary "$SLACK_CHALLENGE_BODY"
[[ "$(cat "$TMP_ROOT/slack-challenge.json")" == "aidocs-slack-challenge" ]] || fail "Slack URL verification challenge was not echoed"
SLACK_DELIVERY="EvAidocsVerify${RUN_ID}"
SLACK_EVENT_BODY="$(jq -cn --arg delivery "$SLACK_DELIVERY" --argjson event_time "$SLACK_NOW" '{type:"event_callback",team_id:"TVERIFY",event_id:$delivery,event_time:$event_time,event:{type:"message",channel:"CVERIFY",user:"UVERIFY",text:"Settlement reconciliation changed for production verification.",ts:"1710000000.000200",thread_ts:"1710000000.000100",event_ts:"1710000000.000200"}}')"
SLACK_EVENT_SIGNATURE="$(slack_signature "$SLACK_NOW" "$SLACK_EVENT_BODY")"
expect_http 202 "$TMP_ROOT/slack-webhook.json" \
	-X POST "$BASE_URL/v1/webhooks/slack" -H 'Content-Type: application/json' \
	-H "X-Slack-Request-Timestamp: $SLACK_NOW" -H "X-Slack-Signature: $SLACK_EVENT_SIGNATURE" \
	--data-binary "$SLACK_EVENT_BODY"
assert_json "$TMP_ROOT/slack-webhook.json" '.accepted == true and .queued == 1'
expect_http 200 "$TMP_ROOT/slack-duplicate.json" \
	-X POST "$BASE_URL/v1/webhooks/slack" -H 'Content-Type: application/json' \
	-H "X-Slack-Request-Timestamp: $SLACK_NOW" -H "X-Slack-Signature: $SLACK_EVENT_SIGNATURE" \
	--data-binary "$SLACK_EVENT_BODY"
assert_json "$TMP_ROOT/slack-duplicate.json" '.duplicate == true'
SLACK_STALE="$((SLACK_NOW - 600))"
SLACK_STALE_SIGNATURE="$(slack_signature "$SLACK_STALE" "$SLACK_EVENT_BODY")"
expect_http 401 "$TMP_ROOT/slack-stale.json" \
	-X POST "$BASE_URL/v1/webhooks/slack" -H 'Content-Type: application/json' \
	-H "X-Slack-Request-Timestamp: $SLACK_STALE" -H "X-Slack-Signature: $SLACK_STALE_SIGNATURE" \
	--data-binary "$SLACK_EVENT_BODY"
wait_for_sync_run "$SLACK_SOURCE_ID" "$SLACK_DELIVERY" "$TMP_ROOT/slack-runs.json"
SLACK_RUN="$(jq -c --arg source "$SLACK_SOURCE_ID" --arg delivery "$SLACK_DELIVERY" '.items[] | select(.source_id == $source and .delivery_id == $delivery)' "$TMP_ROOT/slack-runs.json" | head -n 1)"
[[ "$(printf '%s' "$SLACK_RUN" | jq -r '.outcome')" == "pull_request_opened" ]] || fail "Slack event did not open a pull request"
SLACK_PR_NUMBER="$(printf '%s' "$SLACK_RUN" | jq -er '.pull_request_number')"
owner_get "$TMP_ROOT/slack-pr.json" "/v1/repositories/$REPOSITORY_ID/pull-requests/$SLACK_PR_NUMBER"
SLACK_HEAD_VERSION="$(jq -er '.changes[0].head_version_id' "$TMP_ROOT/slack-pr.json")"
owner_get "$TMP_ROOT/slack-proposal.html" "/v1/documents/$DOCUMENT_ID/versions/$SLACK_HEAD_VERSION/html"
grep -q 'Settlement reconciliation changed' "$TMP_ROOT/slack-proposal.html" || fail "Slack PR did not contain the thread text"
expect_http 200 "$TMP_ROOT/slack-approved.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/pull-requests/$SLACK_PR_NUMBER/reviews" \
	-H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' --data '{"state":"approved","body":"Slack context verified"}'
expect_http 200 "$TMP_ROOT/slack-merged.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/pull-requests/$SLACK_PR_NUMBER/merge" \
	-H "Authorization: Bearer $OWNER_TOKEN"

say "Connect DevRev tickets and verify signed event -> durable run -> aidocs PR"
jq -n --arg document "$DOCUMENT_ID" '{provider:"devrev",external_repository:"webhook-aidocs-verification",target_document_id:$document,trigger_mode:"event",direction:"into_aidocs"}' >"$TMP_ROOT/create-devrev-source.json"
expect_http 201 "$TMP_ROOT/devrev-source.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/sources" \
	-H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
	--data-binary "@$TMP_ROOT/create-devrev-source.json"
DEVREV_SOURCE_ID="$(jq -er '.id' "$TMP_ROOT/devrev-source.json")"
DEVREV_NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
DEVREV_CHALLENGE_BODY="$(jq -cn --arg timestamp "$DEVREV_NOW" '{id:"devrev-verify-challenge",webhook_id:"webhook-aidocs-verification",timestamp:$timestamp,type:"verify",verify:{challenge:"aidocs-devrev-challenge"}}')"
DEVREV_CHALLENGE_SIGNATURE="$(devrev_signature "$DEVREV_CHALLENGE_BODY")"
expect_http 200 "$TMP_ROOT/devrev-challenge.json" \
	-X POST "$BASE_URL/v1/webhooks/devrev" -H 'Content-Type: application/json' \
	-H "X-DevRev-Signature: $DEVREV_CHALLENGE_SIGNATURE" --data-binary "$DEVREV_CHALLENGE_BODY"
assert_json "$TMP_ROOT/devrev-challenge.json" '.challenge == "aidocs-devrev-challenge"'
DEVREV_DELIVERY="devrev-event-${RUN_ID}"
DEVREV_EVENT_BODY="$(jq -cn --arg id "$DEVREV_DELIVERY" --arg timestamp "$DEVREV_NOW" '{id:$id,webhook_id:"webhook-aidocs-verification",timestamp:$timestamp,type:"work_updated",work_updated:{work:{type:"ticket",id:"don:core:verify:ticket/42",display_id:"TKT-42",title:"Settlement runbook is stale",body:"Update the settlement recovery sequence before release.",state:"open"}}}')"
DEVREV_EVENT_SIGNATURE="$(devrev_signature "$DEVREV_EVENT_BODY")"
expect_http 202 "$TMP_ROOT/devrev-webhook.json" \
	-X POST "$BASE_URL/v1/webhooks/devrev" -H 'Content-Type: application/json' \
	-H "X-DevRev-Signature: $DEVREV_EVENT_SIGNATURE" --data-binary "$DEVREV_EVENT_BODY"
assert_json "$TMP_ROOT/devrev-webhook.json" '.accepted == true and .queued == 1'
expect_http 200 "$TMP_ROOT/devrev-duplicate.json" \
	-X POST "$BASE_URL/v1/webhooks/devrev" -H 'Content-Type: application/json' \
	-H "X-DevRev-Signature: $DEVREV_EVENT_SIGNATURE" --data-binary "$DEVREV_EVENT_BODY"
assert_json "$TMP_ROOT/devrev-duplicate.json" '.duplicate == true'
DEVREV_STALE_TIMESTAMP="$(minutes_ago_iso 10)"
DEVREV_STALE_BODY="$(jq -cn --arg id "devrev-stale-${RUN_ID}" --arg timestamp "$DEVREV_STALE_TIMESTAMP" '{id:$id,webhook_id:"webhook-aidocs-verification",timestamp:$timestamp,type:"work_updated",work_updated:{work:{type:"ticket",id:"don:core:verify:ticket/43",display_id:"TKT-43",title:"stale replay"}}}')"
DEVREV_STALE_SIGNATURE="$(devrev_signature "$DEVREV_STALE_BODY")"
expect_http 401 "$TMP_ROOT/devrev-stale.json" \
	-X POST "$BASE_URL/v1/webhooks/devrev" -H 'Content-Type: application/json' \
	-H "X-DevRev-Signature: $DEVREV_STALE_SIGNATURE" --data-binary "$DEVREV_STALE_BODY"
wait_for_sync_run "$DEVREV_SOURCE_ID" "$DEVREV_DELIVERY" "$TMP_ROOT/devrev-runs.json"
DEVREV_RUN="$(jq -c --arg source "$DEVREV_SOURCE_ID" --arg delivery "$DEVREV_DELIVERY" '.items[] | select(.source_id == $source and .delivery_id == $delivery)' "$TMP_ROOT/devrev-runs.json" | head -n 1)"
[[ "$(printf '%s' "$DEVREV_RUN" | jq -r '.outcome')" == "pull_request_opened" ]] || fail "DevRev event did not open a pull request"
DEVREV_PR_NUMBER="$(printf '%s' "$DEVREV_RUN" | jq -er '.pull_request_number')"
owner_get "$TMP_ROOT/devrev-pr.json" "/v1/repositories/$REPOSITORY_ID/pull-requests/$DEVREV_PR_NUMBER"
DEVREV_HEAD_VERSION="$(jq -er '.changes[0].head_version_id' "$TMP_ROOT/devrev-pr.json")"
owner_get "$TMP_ROOT/devrev-proposal.html" "/v1/documents/$DOCUMENT_ID/versions/$DEVREV_HEAD_VERSION/html"
grep -q 'Settlement runbook is stale' "$TMP_ROOT/devrev-proposal.html" || fail "DevRev PR did not contain the ticket title"
grep -q 'Update the settlement recovery sequence' "$TMP_ROOT/devrev-proposal.html" || fail "DevRev PR did not contain the ticket body"
expect_http 200 "$TMP_ROOT/devrev-approved.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/pull-requests/$DEVREV_PR_NUMBER/reviews" \
	-H "Authorization: Bearer $REVIEWER_TOKEN" -H 'Content-Type: application/json' --data '{"state":"approved","body":"DevRev context verified"}'
expect_http 200 "$TMP_ROOT/devrev-merged.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/pull-requests/$DEVREV_PR_NUMBER/merge" \
	-H "Authorization: Bearer $OWNER_TOKEN"

say "Connect a personal DevRev account, verify encrypted persistence, and Sync now to a text PR"
# Keep the preceding unlinked webhook sources as a compatibility proof. Enable
# personal credential storage only after those legacy paths have run.
stop_server
PERSONAL_CONNECTIONS_ENABLED=1
start_server
jq -n --arg token "$DEVREV_PAT" '{personal_access_token:$token}' >"$TMP_ROOT/connect-personal-devrev.json"
expect_http 201 "$TMP_ROOT/personal-devrev-connection.json" \
	-X POST "$BASE_URL/v1/connections/devrev" \
	-H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
	--data-binary "@$TMP_ROOT/connect-personal-devrev.json"
assert_json "$TMP_ROOT/personal-devrev-connection.json" '.provider == "devrev" and .status == "active" and .display_name == "DevRev Verification User" and .workspace_name == "aidocs-production-verification" and .id != ""'
if grep -Fq -- "$DEVREV_PAT" "$TMP_ROOT/personal-devrev-connection.json"; then
	fail "DevRev connection API response exposed the personal access token"
fi
PERSONAL_DEVREV_CONNECTION_ID="$(jq -er '.id' "$TMP_ROOT/personal-devrev-connection.json")"
owner_get "$TMP_ROOT/connections-before-restart.json" "/v1/connections"
assert_json "$TMP_ROOT/connections-before-restart.json" --arg id "$PERSONAL_DEVREV_CONNECTION_ID" '([.connections[] | select(.id == $id and .provider == "devrev" and .status == "active")] | length) == 1'
if grep -Fq -- "$DEVREV_PAT" "$TMP_ROOT/connections-before-restart.json"; then
	fail "connected-account list exposed the DevRev personal access token"
fi

PERSONAL_DEVREV_CIPHERTEXT="$(psql "$DATABASE_URL" -Atqc "SELECT encode(credential_ciphertext,'hex') FROM connected_accounts WHERE id='$PERSONAL_DEVREV_CONNECTION_ID'")"
[[ ${#PERSONAL_DEVREV_CIPHERTEXT} -gt 64 ]] || fail "DevRev credential ciphertext was not persisted"
PERSONAL_DEVREV_PAT_HEX="$(printf '%s' "$DEVREV_PAT" | od -An -v -tx1 | tr -d ' \n')"
PERSONAL_DEVREV_PLAINTEXT_POSITION="$(psql "$DATABASE_URL" -Atqc "SELECT position(decode('$PERSONAL_DEVREV_PAT_HEX','hex') in credential_ciphertext) FROM connected_accounts WHERE id='$PERSONAL_DEVREV_CONNECTION_ID'")"
[[ "$PERSONAL_DEVREV_PLAINTEXT_POSITION" == "0" ]] || fail "DevRev credential ciphertext contains the plaintext PAT"
PERSONAL_DEVREV_ACCOUNT_TEXT="$(psql "$DATABASE_URL" -Atqc "SELECT concat_ws('|',id,user_id,provider,external_account_id,display_name,workspace_id,workspace_name,scopes::text,status,coalesce(expires_at::text,'')) FROM connected_accounts WHERE id='$PERSONAL_DEVREV_CONNECTION_ID'")"
if grep -Fq -- "$DEVREV_PAT" <<<"$PERSONAL_DEVREV_ACCOUNT_TEXT"; then
	fail "DevRev PAT was persisted in a connected-account text column"
fi

jq -n \
	--arg connection "$PERSONAL_DEVREV_CONNECTION_ID" \
	--arg document "$DOCUMENT_ID" \
	'{connection_id:$connection,provider:"devrev",external_repository:"TKT-9001",target_document_id:$document,trigger_mode:"manual",direction:"into_aidocs"}' \
	>"$TMP_ROOT/create-personal-devrev-source.json"
expect_http 201 "$TMP_ROOT/personal-devrev-source.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/sources" \
	-H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' \
	--data-binary "@$TMP_ROOT/create-personal-devrev-source.json"
assert_json "$TMP_ROOT/personal-devrev-source.json" --arg connection "$PERSONAL_DEVREV_CONNECTION_ID" '.provider == "devrev" and .connection_id == $connection and .connection.status == "active"'
PERSONAL_DEVREV_SOURCE_ID="$(jq -er '.id' "$TMP_ROOT/personal-devrev-source.json")"
expect_http 200 "$TMP_ROOT/personal-devrev-sync.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/sources/$PERSONAL_DEVREV_SOURCE_ID/check" \
	-H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' --data '{}'
assert_json "$TMP_ROOT/personal-devrev-sync.json" '.outcome == "pull_request_opened" and (.after_sha | startswith("sha256:")) and .changed_files == ["devrev/works/TKT-9001.txt"]'
PERSONAL_DEVREV_REVISION="$(jq -er '.after_sha' "$TMP_ROOT/personal-devrev-sync.json")"
PERSONAL_DEVREV_PR_NUMBER="$(jq -er '.pull_request_number' "$TMP_ROOT/personal-devrev-sync.json")"
owner_get "$TMP_ROOT/personal-devrev-pr.json" "/v1/repositories/$REPOSITORY_ID/pull-requests/$PERSONAL_DEVREV_PR_NUMBER"
PERSONAL_DEVREV_HEAD_VERSION="$(jq -er '.changes[0].head_version_id' "$TMP_ROOT/personal-devrev-pr.json")"
owner_get "$TMP_ROOT/personal-devrev-proposal.html" "/v1/documents/$DOCUMENT_ID/versions/$PERSONAL_DEVREV_HEAD_VERSION/html"
grep -q 'Personal DevRev knowledge is stale' "$TMP_ROOT/personal-devrev-proposal.html" || fail "personal DevRev PR did not contain the ticket title"
grep -q 'Refresh the settlement recovery steps' "$TMP_ROOT/personal-devrev-proposal.html" || fail "personal DevRev PR did not contain the ticket body"
for output in "$TMP_ROOT/personal-devrev-source.json" "$TMP_ROOT/personal-devrev-sync.json" "$TMP_ROOT/personal-devrev-pr.json" "$TMP_ROOT/personal-devrev-proposal.html" "$SERVER_LOG" "$DEVREV_FIXTURE_LOG"; do
	if grep -Fq -- "$DEVREV_PAT" "$output"; then
		fail "personal DevRev PAT leaked into $(basename "$output")"
	fi
done
grep -q 'POST /auth-tokens.info' "$DEVREV_FIXTURE_LOG" || fail "personal DevRev connection did not validate the PAT"
grep -q 'POST /works.get' "$DEVREV_FIXTURE_LOG" || fail "personal DevRev Sync now did not inspect the work item"

say "Restart the production server and prove PostgreSQL/S3 persistence"
owner_get "$TMP_ROOT/document-before-restart.json" "/v1/documents/$DOCUMENT_ID"
FINAL_VERSION_ID="$(jq -er '.current_version_id' "$TMP_ROOT/document-before-restart.json")"
stop_server
start_server
owner_get "$TMP_ROOT/connections-after-restart.json" "/v1/connections"
assert_json "$TMP_ROOT/connections-after-restart.json" --arg id "$PERSONAL_DEVREV_CONNECTION_ID" '([.connections[] | select(.id == $id and .provider == "devrev" and .status == "active")] | length) == 1'
if grep -Fq -- "$DEVREV_PAT" "$TMP_ROOT/connections-after-restart.json"; then
	fail "restarted connected-account list exposed the DevRev personal access token"
fi
PERSONAL_DEVREV_CIPHERTEXT_AFTER_RESTART="$(psql "$DATABASE_URL" -Atqc "SELECT encode(credential_ciphertext,'hex') FROM connected_accounts WHERE id='$PERSONAL_DEVREV_CONNECTION_ID'")"
[[ "$PERSONAL_DEVREV_CIPHERTEXT_AFTER_RESTART" == "$PERSONAL_DEVREV_CIPHERTEXT" ]] || fail "DevRev credential ciphertext changed across restart"
expect_http 200 "$TMP_ROOT/personal-devrev-sync-after-restart.json" \
	-X POST "$BASE_URL/v1/repositories/$REPOSITORY_ID/sources/$PERSONAL_DEVREV_SOURCE_ID/check" \
	-H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' --data '{}'
assert_json "$TMP_ROOT/personal-devrev-sync-after-restart.json" --arg revision "$PERSONAL_DEVREV_REVISION" '.outcome == "no_op" and .after_sha == $revision'
for output in "$TMP_ROOT/connections-after-restart.json" "$TMP_ROOT/personal-devrev-sync-after-restart.json" "$SERVER_LOG" "$DEVREV_FIXTURE_LOG"; do
	if grep -Fq -- "$DEVREV_PAT" "$output"; then
		fail "post-restart DevRev flow exposed the personal access token in $(basename "$output")"
	fi
done
owner_get "$TMP_ROOT/repository-after-restart.json" "/v1/repositories/$REPOSITORY_ID"
assert_json "$TMP_ROOT/repository-after-restart.json" '.slug == "production-handbook" and .scope == "organization"'
owner_get "$TMP_ROOT/document-after-restart.json" "/v1/documents/$DOCUMENT_ID"
[[ "$(jq -er '.current_version_id' "$TMP_ROOT/document-after-restart.json")" == "$FINAL_VERSION_ID" ]] || fail "document head changed across restart"
owner_get "$TMP_ROOT/final-html.html" "/v1/documents/$DOCUMENT_ID/versions/$FINAL_VERSION_ID/html"
grep -q 'Settlement reconciliation changed' "$TMP_ROOT/final-html.html" || fail "Slack context did not persist across restart"
grep -q 'Settlement runbook is stale' "$TMP_ROOT/final-html.html" || fail "DevRev context did not persist across restart"
owner_get "$TMP_ROOT/sources-after-restart.json" "/v1/repositories/$REPOSITORY_ID/sources"
assert_json "$TMP_ROOT/sources-after-restart.json" --arg connection "$PERSONAL_DEVREV_CONNECTION_ID" '([.items[] | select(.provider == "slack")] | length) == 1 and ([.items[] | select(.provider == "devrev")] | length) == 2 and ([.items[] | select(.connection_id == $connection)] | length) == 1 and ([.items[] | select(.provider == "gitlab" or .provider == "bitbucket")] | length) == 0'

MIGRATION_VERSION="$(psql "$DATABASE_URL" -Atqc 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')"
[[ "$MIGRATION_VERSION" -ge 25 ]] || fail "expected repository, integrity, and connected-account migrations, got $MIGRATION_VERSION"
STUCK_RUNS="$(psql "$DATABASE_URL" -Atqc "SELECT count(*) FROM connected_knowledge_records WHERE kind='sync_run' AND payload->>'status' IN ('queued','processing')")"
[[ "$STUCK_RUNS" == "0" ]] || fail "$STUCK_RUNS durable sync runs remained stuck"
SOURCE_COUNT="$(psql "$DATABASE_URL" -Atqc "SELECT count(*) FROM connected_knowledge_records WHERE kind='source' AND repository_id='$REPOSITORY_ID'")"
[[ "$SOURCE_COUNT" == "3" ]] || fail "expected 3 persisted connected sources, got $SOURCE_COUNT"
OBJECT_COUNT="$(minio_object_count)"
[[ "$OBJECT_COUNT" -ge 6 ]] || fail "expected immutable version objects in S3, got $OBJECT_COUNT"

say "Prove readiness fails closed when blob storage is unavailable"
docker pause "$MINIO_NAME" >/dev/null
expect_http 503 "$TMP_ROOT/readiness-without-blob.json" "$BASE_URL/readyz"
assert_json "$TMP_ROOT/readiness-without-blob.json" '.status == "error" and .checks.blob == "error"'
docker unpause "$MINIO_NAME" >/dev/null
wait_for "readiness recovery" curl --fail --silent "$BASE_URL/readyz"

if [[ "$VERIFY_IMAGE" == "1" ]]; then
	say "Build and boot the actual Dockerfile artifact with Helm-equivalent restrictions"
	docker build \
		--tag "$IMAGE_TAG" \
		--build-arg AIDOCS_VERSION=production-verification \
		--build-arg AIDOCS_COMMIT_SHA="$RUN_ID" \
		"$ROOT_DIR"
	IMAGE_APP_PORT="$(free_port)"
	IMAGE_METRICS_PORT="$(free_port)"
	IMAGE_BASE_URL="http://127.0.0.1:$IMAGE_APP_PORT"
	docker run --detach --name "$IMAGE_NAME" --network "$NETWORK_NAME" \
		--read-only \
		--user 65532:65532 \
		--cap-drop ALL \
		--security-opt no-new-privileges:true \
		--tmpfs /tmp:rw,nosuid,nodev,noexec,size=128m,uid=65532,gid=65532,mode=1770 \
		--publish "127.0.0.1:$IMAGE_APP_PORT:8080" \
		--publish "127.0.0.1:$IMAGE_METRICS_PORT:9090" \
		--env "DATABASE_URL=postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@$POSTGRES_NAME:5432/$POSTGRES_DATABASE?sslmode=disable" \
		--env "BLOB_BUCKET=$BLOB_BUCKET" \
		--env BLOB_REGION=us-east-1 \
		--env "BLOB_ENDPOINT=http://$MINIO_NAME:9000" \
		--env BLOB_FORCE_PATH_STYLE=true \
		--env "BLOB_ACCESS_KEY_ID=$MINIO_USER" \
		--env "BLOB_SECRET_ACCESS_KEY=$MINIO_PASSWORD" \
		--env AIDOCS_PORT=8080 \
		--env AIDOCS_METRICS_PORT=9090 \
		--env "AIDOCS_APP_ORIGIN=$IMAGE_BASE_URL" \
		--env "AIDOCS_RENDER_ORIGIN=http://localhost:$IMAGE_APP_PORT" \
		--env AIDOCS_DEPLOYMENT=org \
		--env AIDOCS_ORG_DOMAINS=example.com \
		--env 'AIDOCS_ORG_NAME=Production Verification' \
		--env AIDOCS_MIGRATE=true \
		--env AIDOCS_SOURCE_POLL_INTERVAL=0 \
		--env "AIDOCS_CONNECTION_ENCRYPTION_KEY=$CONNECTION_ENCRYPTION_KEY" \
		--env "AIDOCS_OIDC_ISSUER_URL=$OIDC_ISSUER" \
		--env "AIDOCS_OIDC_INTERNAL_ISSUER_URL=http://$DEX_NAME:5556/dex" \
		--env AIDOCS_OIDC_CLIENT_ID=aidocs-production-verification \
		--env AIDOCS_OIDC_CLIENT_SECRET=aidocs-production-verification-oidc-secret \
		--env 'AIDOCS_OIDC_PROVIDER_NAME=Dex verification' \
		--env SESSION_SECRET=aidocs-production-verification-session-secret \
		--env AIDOCS_GITHUB_WEBHOOK_SECRET=aidocs-github-verification-secret \
		--env "AIDOCS_SLACK_SIGNING_SECRET=$SLACK_SECRET" \
		--env "AIDOCS_DEVREV_WEBHOOK_SECRET=$DEVREV_SECRET" \
		"$IMAGE_TAG" >/dev/null
	wait_for "restricted production image readiness" curl --fail --silent "$IMAGE_BASE_URL/readyz"
	IMAGE_SECURITY="$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}|{{.Config.User}}|{{index .HostConfig.Tmpfs "/tmp"}}|{{json .HostConfig.CapDrop}}|{{json .HostConfig.SecurityOpt}}' "$IMAGE_NAME")"
	[[ "$IMAGE_SECURITY" == true\|65532:65532\|*uid=65532*gid=65532* ]] || fail "actual image did not boot with expected read-only/non-root/tmpfs restrictions: $IMAGE_SECURITY"
	[[ "$IMAGE_SECURITY" == *size=128m* || "$IMAGE_SECURITY" == *size=134217728* ]] || fail "actual image /tmp was not bounded to 128 MiB: $IMAGE_SECURITY"
	[[ "$IMAGE_SECURITY" == *'["ALL"]'* ]] || fail "actual image did not drop every Linux capability: $IMAGE_SECURITY"
	[[ "$IMAGE_SECURITY" == *'["no-new-privileges:true"]'* || "$IMAGE_SECURITY" == *'["no-new-privileges"]'* ]] || fail "actual image did not disable privilege escalation: $IMAGE_SECURITY"
	expect_http 200 "$TMP_ROOT/image-frontend.html" "$IMAGE_BASE_URL/app/repositories"
	grep -q '<div id="root"></div>' "$TMP_ROOT/image-frontend.html" || fail "actual image did not serve its embedded frontend"
	expect_http 200 "$TMP_ROOT/image-repository.json" \
		-H "Authorization: Bearer $OWNER_TOKEN" "$IMAGE_BASE_URL/v1/repositories/$REPOSITORY_ID"
	assert_json "$TMP_ROOT/image-repository.json" '.slug == "production-handbook"'
	expect_http 200 "$TMP_ROOT/image-connections.json" \
		-H "Authorization: Bearer $OWNER_TOKEN" "$IMAGE_BASE_URL/v1/connections"
	assert_json "$TMP_ROOT/image-connections.json" --arg id "$PERSONAL_DEVREV_CONNECTION_ID" '([.connections[] | select(.id == $id and .provider == "devrev" and .status == "active")] | length) == 1'
	if grep -Fq -- "$DEVREV_PAT" "$TMP_ROOT/image-connections.json"; then
		fail "actual image exposed the persisted DevRev personal access token"
	fi
	docker logs "$IMAGE_NAME" >"$TMP_ROOT/image-server.log" 2>&1
	if grep -Fq -- "$DEVREV_PAT" "$TMP_ROOT/image-server.log"; then
		fail "actual image logged the persisted DevRev personal access token"
	fi

	# ParseMultipartForm keeps only 8 MiB of file content in memory. These two
	# files deliberately cross that aggregate threshold, proving the restricted
	# container has the same bounded writable /tmp that Helm mounts in production.
	mkdir -p "$TMP_ROOT/image-import"
	{
		printf '%s' '<!doctype html><html><body><p>image import one</p>'
		head -c 5242880 /dev/zero | tr '\0' 'a'
		printf '%s' '</body></html>'
	} >"$TMP_ROOT/image-import/large-one.html"
	{
		printf '%s' '<!doctype html><html><body><p>image import two</p>'
		head -c 5242880 /dev/zero | tr '\0' 'b'
		printf '%s' '</body></html>'
	} >"$TMP_ROOT/image-import/large-two.html"
	IMAGE_OBJECTS_BEFORE="$(minio_object_count)"
	expect_http 201 "$TMP_ROOT/image-large-import.json" \
		-X POST "$IMAGE_BASE_URL/v1/repositories/import" \
		-H "Authorization: Bearer $OWNER_TOKEN" \
		-F 'name=Restricted image large import' \
		-F "slug=restricted-image-large-import-$RUN_ID" \
		-F 'scope=personal' \
		-F 'import_kind=folder' \
		-F 'paths=large-one.html' \
		-F "files=@$TMP_ROOT/image-import/large-one.html;filename=large-one.html;type=text/html" \
		-F 'paths=nested/large-two.html' \
		-F "files=@$TMP_ROOT/image-import/large-two.html;filename=large-two.html;type=text/html"
	assert_json "$TMP_ROOT/image-large-import.json" '(.documents | length) == 2'
	IMAGE_IMPORT_REPOSITORY_ID="$(jq -er '.repository.id' "$TMP_ROOT/image-large-import.json")"
	expect_http 204 "$TMP_ROOT/image-large-import-delete.json" \
		-X DELETE -H "Authorization: Bearer $OWNER_TOKEN" \
		"$IMAGE_BASE_URL/v1/repositories/$IMAGE_IMPORT_REPOSITORY_ID"
	IMAGE_OBJECTS_AFTER="$(minio_object_count)"
	[[ "$IMAGE_OBJECTS_AFTER" == "$IMAGE_OBJECTS_BEFORE" ]] || fail "actual-image import cleanup changed durable object count ($IMAGE_OBJECTS_BEFORE -> $IMAGE_OBJECTS_AFTER)"
else
	say "Skip actual Dockerfile artifact boot (run make verify-production-image to enable it)"
fi

printf '\nproduction verification: PASS\n'
printf '  repository: %s\n' "$REPOSITORY_ID"
printf '  imported documents: 3\n'
printf '  governed pull requests merged: 3\n'
printf '  personal connected-account PR opened: 1\n'
printf '  connected sources: %s\n' "$SOURCE_COUNT"
printf '  immutable S3 objects: %s\n' "$OBJECT_COUNT"
printf '  migration version: %s\n' "$MIGRATION_VERSION"
printf '  actual Docker image: %s\n' "$([[ "$VERIFY_IMAGE" == "1" ]] && printf verified || printf skipped)"
