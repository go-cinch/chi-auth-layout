#!/usr/bin/env bash
# Regenerate and run local Auth/Vben demos; see ../LOCAL_DEV_WORKFLOW.md.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: local-debug.sh [options]

Regenerate demos/auth and demos/vben-antd-vue3 from the current local templates,
then start both services in the background. Existing services are restarted.

Options (all cleanup switches default to false):
  --stop-auth                     Stop only Auth (8081), then exit; other actions are skipped.
  --clean-sql[=true|false]          Replace generated SQL with the latest template SQL.
  --clean-credentials[=true|false]  Replace backend authentication credentials/key files.
  --clean-auth[=true|false]         Drop/recreate ONLY the local pg container's auth database.
  --skip-tests                     Skip the backend generation hook's Go tests.
  --dry-run                       Print the plan without stopping or changing anything.
  -h, --help                      Show this help.

Aliases: --delete-old-sql, --delete-old-credentials, --reset-auth-db.

Examples:
  ./local-debug.sh
  ./local-debug.sh --stop-auth
  ./local-debug.sh --clean-sql --clean-auth
  ./local-debug.sh --clean-sql --clean-credentials --clean-auth
  ./local-debug.sh --clean-sql=true --clean-credentials=false --clean-auth=true

SQL and database cleanup are independent. Replacing SQL while retaining an old
database can cause migration errors; the script never silently resets that database.
Credential cleanup affects the backend only; Vben's store key is always preserved.
Retained MFA bindings need their original encryption key. Other backend settings
use the template defaults. Redis is not cleared. No git pull/commit/push is performed.
EOF
}

fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }
clean_sql=false
clean_credentials=false
clean_auth=false
skip_tests=false
dry_run=false
stop_auth=false
for arg in "$@"; do
  case "$arg" in
    -h|--help) usage; exit 0 ;;
    --clean-sql|--delete-old-sql) clean_sql=true ;;
    --clean-credentials|--delete-old-credentials) clean_credentials=true ;;
    --clean-auth|--reset-auth-db) clean_auth=true ;;
    --clean-sql=*|--delete-old-sql=*) clean_sql=${arg#*=} ;;
    --clean-credentials=*|--delete-old-credentials=*) clean_credentials=${arg#*=} ;;
    --clean-auth=*|--reset-auth-db=*) clean_auth=${arg#*=} ;;
    --stop-auth) stop_auth=true ;;
    --skip-tests) skip_tests=true ;;
    --dry-run) dry_run=true ;;
    *) fail "Unknown argument: $arg (use --help)" ;;
  esac
done
for value in "$clean_sql" "$clean_credentials" "$clean_auth"; do
  case "$value" in true|false) ;; *) fail 'Cleanup values must be true or false' ;; esac
done

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
BACKEND_TEMPLATE=$(cd "$SCRIPT_DIR/.." && pwd -P)
ROOT=$(cd "$BACKEND_TEMPLATE/.." && pwd -P)
FRONTEND_TEMPLATE="$ROOT/pc-admin-fe-layout"
AUTH_DIR="$ROOT/demos/auth"
VBEN_DIR="$ROOT/demos/vben-antd-vue3"

stop_service() {
  local port=$1 expected_dir=$2 pid process_dir attempt
  local pids
  pids=$(lsof -tiTCP:"$port" -sTCP:LISTEN || true)
  for pid in $pids; do
    process_dir=$(lsof -a -p "$pid" -d cwd -Fn | sed -n 's/^n//p')
    case "$process_dir" in
      "$expected_dir"|"$expected_dir"/*) ;;
      *) fail "Port $port belongs to PID $pid outside $expected_dir; left untouched" ;;
    esac
  done
  for pid in $pids; do kill -TERM "$pid"; done
  for attempt in {1..30}; do
    if ! lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null; then return; fi
    sleep 1
  done
  fail "Port $port did not stop within 30 seconds"
}


# Stop-only mode takes precedence over generation and cleanup switches.
if [ "$stop_auth" = true ]; then
  if [ "$dry_run" = true ]; then
    printf 'Stop only Auth in %s (port 8081), then exit.\n' "$AUTH_DIR"
    exit 0
  fi
  command -v lsof >/dev/null || fail 'Missing command: lsof'
  stop_service 8081 "$AUTH_DIR"
  printf 'Auth stopped (port 8081).\n'
  exit 0
fi

printf 'Backend template: %s\nFrontend template: %s\n' "$BACKEND_TEMPLATE" "$FRONTEND_TEMPLATE"
printf 'Replace SQL: %s | Replace backend credentials: %s | Reset auth DB: %s\n' \
  "$clean_sql" "$clean_credentials" "$clean_auth"
if [ "$dry_run" = true ]; then
  printf '1. Stop only Auth/Vben listeners in the corresponding demos (8081/5666).\n'
  printf '2. SCAFFOLD_SKIP_TESTS=%s make -C "%s" local DELETE_OLD_SQL=%s DELETE_OLD_CREDENTIALS=%s\n' \
    "$skip_tests" "$BACKEND_TEMPLATE" "$clean_sql" "$clean_credentials"
  printf '3. make -C "%s" local DEMO=vben-antd-vue3 UI=vben-antd-vue3\n' "$FRONTEND_TEMPLATE"
  printf '4. pnpm install --frozen-lockfile in %s\n' "$VBEN_DIR"
  if [ "$clean_auth" = true ]; then
    printf '5. Verify loopback auth DSN; docker exec pg dropdb -U root --force --if-exists auth\n'
    printf '   docker exec pg createdb -U root auth\n'
  else
    printf '5. Keep the existing auth database.\n'
  fi
  printf '6. Start ./bin/auth -c ./conf and pnpm dev; wait for health and proxy checks.\n'
  exit 0
fi

for command in make scaffold go pnpm python3 lsof curl git; do
  command -v "$command" >/dev/null || fail "Missing command: $command"
done
python3 -c 'import yaml' || fail 'Install PyYAML using scripts/requirements-local.txt'
test -f "$FRONTEND_TEMPLATE/Makefile" || fail "Missing frontend template: $FRONTEND_TEMPLATE"
for repo in "$BACKEND_TEMPLATE" "$FRONTEND_TEMPLATE" "$ROOT/demos"; do
  if git -C "$repo" rev-parse --git-dir >/dev/null 2>&1; then
    test -z "$(git -C "$repo" diff --name-only --diff-filter=U)" || fail "Unmerged files in $repo"
  fi
done

# Prevent simultaneous invocations from regenerating the same demos.
LOCK_DIR="$BACKEND_TEMPLATE/.local-debug.lock"
mkdir "$LOCK_DIR" 2>/dev/null || fail "Another run is active (lock: $LOCK_DIR)"
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
LOG_DIR=$(mktemp -d "${TMPDIR:-/tmp}/go-cinch-local-debug.XXXXXX")
printf 'Logs: %s\n' "$LOG_DIR"

verify_database() {
  python3 - "$AUTH_DIR/conf/database.yml" <<'PY'
import os, pathlib, sys, urllib.parse, yaml
path = pathlib.Path(sys.argv[1])
if not path.exists():
    sys.exit('Missing generated database configuration')
config = yaml.safe_load(path.read_text())['database']
dsn = os.environ.get('SERVICE_DATABASE_DSN', config['dsn'])
url = urllib.parse.urlparse(dsn)
if (url.scheme not in ('postgres', 'postgresql') or url.hostname != '127.0.0.1'
        or url.port != 5432 or url.path != '/auth' or url.username != 'root'):
    sys.exit('Refusing database reset: effective DSN must target root@127.0.0.1:5432/auth')
PY
}

if [ "$clean_auth" = true ]; then
  command -v docker >/dev/null || fail 'Missing command: docker'
  docker exec pg pg_isready -U root -d postgres >/dev/null
  docker port pg 5432/tcp | grep -Eq '^(0\.0\.0\.0|127\.0\.0\.1):5432$' \
    || fail 'Container pg must publish PostgreSQL on local port 5432'
  if [ -f "$AUTH_DIR/conf/database.yml" ]; then verify_database; fi
fi


run_step() {
  local label=$1 log=$2
  shift 2
  printf '\n%s (log: %s)\n' "$label" "$log"
  if "$@" >"$log" 2>&1; then return; fi
  tail -n 40 "$log" >&2
  fail "$label failed; see $log"
}

stop_service 8081 "$AUTH_DIR"
stop_service 5666 "$VBEN_DIR"
run_step 'Regenerating Auth' "$LOG_DIR/auth-generate.log" \
  env SCAFFOLD_SKIP_TESTS="$skip_tests" make -C "$BACKEND_TEMPLATE" local \
  DELETE_OLD_SQL="$clean_sql" DELETE_OLD_CREDENTIALS="$clean_credentials"
run_step 'Regenerating Vben PC' "$LOG_DIR/vben-generate.log" \
  make -C "$FRONTEND_TEMPLATE" local DEMO=vben-antd-vue3 UI=vben-antd-vue3

# Preserve all other ignored environment settings, updating only local routing.
python3 - "$VBEN_DIR/apps/web-antd/.env.development.local" <<'PY'
import pathlib, sys
path = pathlib.Path(sys.argv[1])
settings = {'AUTH_PROXY_TARGET': 'http://127.0.0.1:8081', 'VITE_PORT': '5666',
            '__VITE_ADDITIONAL_SERVER_ALLOWED_HOSTS': 'preview-local.go-cinch.top'}
lines = path.read_text().splitlines() if path.exists() else []
result = [line for line in lines if line.split('=', 1)[0].strip() not in settings]
result.extend(f'{key}={value}' for key, value in settings.items())
path.write_text('\n'.join(result) + '\n')
PY
run_step 'Installing Vben dependencies' "$LOG_DIR/vben-install.log" \
  pnpm --dir "$VBEN_DIR" install --frozen-lockfile

if [ "$clean_auth" = true ]; then
  verify_database
  printf '\nRecreating local pg/auth database...\n'
  docker exec pg dropdb -U root --force --if-exists auth
  docker exec pg createdb -U root auth
fi

# A separate session and redirected streams keep services alive after this script exits.
start_service() {
  python3 - "$@" <<'PY'
import pathlib, subprocess, sys
cwd, log_path, pid_path, *command = sys.argv[1:]
with open(log_path, 'ab') as log:
    process = subprocess.Popen(command, cwd=cwd, stdin=subprocess.DEVNULL,
                               stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
pathlib.Path(pid_path).write_text(str(process.pid) + '\n')
print(f'Started PID {process.pid}; log: {log_path}')
PY
}

wait_http() {
  local url=$1 pid_file=$2 log=$3 attempt pid
  pid=$(cat "$pid_file")
  for attempt in {1..90}; do
    if ! kill -0 "$pid" 2>/dev/null; then
      tail -n 40 "$log" >&2
      fail "Service exited before becoming ready; see $log"
    fi
    if curl --fail --silent --max-time 2 "$url" >/dev/null; then return; fi
    sleep 1
  done
  fail "Timed out waiting for $url; see $log (service was left running)"
}

start_service "$AUTH_DIR" "$LOG_DIR/auth.log" "$LOG_DIR/auth.pid" ./bin/auth -c ./conf
wait_http http://127.0.0.1:8081/healthz "$LOG_DIR/auth.pid" "$LOG_DIR/auth.log"
start_service "$VBEN_DIR" "$LOG_DIR/vben.log" "$LOG_DIR/vben.pid" pnpm dev
wait_http http://127.0.0.1:5666/ "$LOG_DIR/vben.pid" "$LOG_DIR/vben.log"
wait_http http://127.0.0.1:5666/api/auth/pub/opt "$LOG_DIR/vben.pid" "$LOG_DIR/vben.log"
printf '\nReady: Auth http://127.0.0.1:8081 | Vben PC http://127.0.0.1:5666\nLogs: %s\n' "$LOG_DIR"
