#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/aidocs-launcher-test.XXXXXX")"
TMP_ROOT="$(CDPATH= cd -- "$TMP_ROOT" && pwd)"
trap 'rm -rf "$TMP_ROOT"' EXIT

fail() {
	printf 'launcher test: %s\n' "$*" >&2
	exit 1
}

assert_contains() {
	local file="$1"
	local expected="$2"
	grep -Fq "$expected" "$file" || fail "expected $file to contain: $expected"
}

assert_not_contains() {
	local file="$1"
	local unexpected="$2"
	if grep -Fq "$unexpected" "$file"; then
		fail "expected $file not to contain: $unexpected"
	fi
}

assert_absent() {
	[[ ! -e "$1" ]] || fail "expected path to be removed: $1"
}

PROJECT="$TMP_ROOT/project"
FAKE_BIN="$TMP_ROOT/fake-bin"
LOG="$TMP_ROOT/commands.log"
mkdir -p \
	"$PROJECT/frontend" \
	"$PROJECT/api/cmd/aidocs-dev" \
	"$PROJECT/api/internal/server/frontend_dist/assets" \
	"$FAKE_BIN"
cp "$ROOT_DIR/aidocs-dev" "$PROJECT/aidocs-dev"
chmod +x "$PROJECT/aidocs-dev"

cat >"$FAKE_BIN/npm" <<'EOF'
#!/usr/bin/env bash
printf 'npm|%s|%s\n' "$PWD" "$*" >>"$AIDOCS_LAUNCHER_TEST_LOG"
EOF
cat >"$FAKE_BIN/go" <<'EOF'
#!/usr/bin/env bash
printf 'go|%s|%s\n' "$PWD" "$*" >>"$AIDOCS_LAUNCHER_TEST_LOG"
EOF
chmod +x "$FAKE_BIN/npm" "$FAKE_BIN/go"
ln -s "$PROJECT/aidocs-dev" "$FAKE_BIN/aidocs-dev"

HELP_OUTPUT="$TMP_ROOT/help.txt"
PATH="$FAKE_BIN:$PATH" aidocs-dev help >"$HELP_OUTPUT"
assert_contains "$HELP_OUTPUT" "aidocs-dev [start]"
assert_contains "$HELP_OUTPUT" "does not use Docker Compose"

: >"$LOG"
PATH="$FAKE_BIN:$PATH" \
	AIDOCS_LAUNCHER_TEST_LOG="$LOG" \
	AIDOCS_DEV_TAG="fixturetag" \
	AIDOCS_DEV_PACKAGE="./api/cmd/aidocs-dev" \
	AIDOCS_DEV_PORT="18080" \
	AIDOCS_DEV_NPM_INSTALL="auto" \
	AIDOCS_DEV_SKIP_FRONTEND="" \
	"$PROJECT/aidocs-dev" >"$TMP_ROOT/dev-output.txt"
assert_contains "$LOG" "npm|$PROJECT/frontend|ci"
assert_contains "$LOG" "npm|$PROJECT/frontend|run build"
assert_contains "$LOG" "go|$PROJECT|run -tags fixturetag ./api/cmd/aidocs-dev"
assert_contains "$TMP_ROOT/dev-output.txt" "http://127.0.0.1:18080"
assert_contains "$TMP_ROOT/dev-output.txt" "http://127.0.0.1:18080/__dev/login?as=owner&redirect=/app/repositories"
assert_contains "$TMP_ROOT/dev-output.txt" "http://127.0.0.1:18080/__dev/login?as=reviewer&redirect=/app/repositories"

mkdir -p "$PROJECT/frontend/node_modules"
touch "$PROJECT/frontend/package-lock.json" "$PROJECT/frontend/node_modules/.package-lock.json"
: >"$LOG"
PATH="$FAKE_BIN:$PATH" \
	AIDOCS_LAUNCHER_TEST_LOG="$LOG" \
	AIDOCS_DEV_TAG="fixturetag" \
	AIDOCS_DEV_PACKAGE="./api/cmd/aidocs-dev" \
	AIDOCS_DEV_NPM_INSTALL="auto" \
	"$PROJECT/aidocs-dev" >"$TMP_ROOT/fresh-lock-output.txt"
assert_not_contains "$LOG" "npm|$PROJECT/frontend|ci"
assert_contains "$LOG" "npm|$PROJECT/frontend|run build"

touch -t 202001010000 "$PROJECT/frontend/node_modules/.package-lock.json"
: >"$LOG"
PATH="$FAKE_BIN:$PATH" \
	AIDOCS_LAUNCHER_TEST_LOG="$LOG" \
	AIDOCS_DEV_TAG="fixturetag" \
	AIDOCS_DEV_PACKAGE="./api/cmd/aidocs-dev" \
	AIDOCS_DEV_NPM_INSTALL="auto" \
	"$PROJECT/aidocs-dev" >"$TMP_ROOT/stale-lock-output.txt"
assert_contains "$LOG" "npm|$PROJECT/frontend|ci"

UNKNOWN_OUTPUT="$TMP_ROOT/unknown.txt"
set +e
"$PROJECT/aidocs-dev" unknown >"$UNKNOWN_OUTPUT" 2>&1
UNKNOWN_STATUS=$?
set -e
[[ "$UNKNOWN_STATUS" -eq 2 ]] || fail "unknown command status = $UNKNOWN_STATUS, want 2"
assert_contains "$UNKNOWN_OUTPUT" "unknown launcher command"

mkdir -p "$PROJECT/bin" "$PROJECT/frontend/dist"
touch \
	"$PROJECT/bin/aidocs-dev" \
	"$PROJECT/frontend/dist/index.html" \
	"$PROJECT/frontend/tsconfig.tsbuildinfo" \
	"$PROJECT/frontend/tsconfig.node.tsbuildinfo" \
	"$PROJECT/frontend/vite.config.js" \
	"$PROJECT/frontend/vite.config.d.ts" \
	"$PROJECT/api/internal/server/frontend_dist/index.html" \
	"$PROJECT/api/internal/server/frontend_dist/favicon.svg" \
	"$PROJECT/api/internal/server/frontend_dist/assets/app.js" \
	"$PROJECT/api/internal/server/frontend_dist/assets/.gitkeep"
: >"$LOG"
PATH="$FAKE_BIN:$PATH" \
	AIDOCS_LAUNCHER_TEST_LOG="$LOG" \
	"$PROJECT/aidocs-dev" clean >"$TMP_ROOT/clean-output.txt"
assert_absent "$PROJECT/bin"
assert_absent "$PROJECT/frontend/dist"
assert_absent "$PROJECT/frontend/tsconfig.tsbuildinfo"
assert_absent "$PROJECT/api/internal/server/frontend_dist/index.html"
assert_absent "$PROJECT/api/internal/server/frontend_dist/assets/app.js"
[[ -f "$PROJECT/api/internal/server/frontend_dist/assets/.gitkeep" ]] || fail "clean removed the tracked assets placeholder"
assert_contains "$LOG" "go|$PROJECT|clean -testcache"

printf 'launcher test: PASS\n'
