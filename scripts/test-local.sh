#!/bin/sh

set -eu

template_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/chi-auth-local-test.XXXXXX")
trap 'rm -rf "$test_root"' EXIT INT TERM

# Exercise the real Make recipe in isolation; never scaffold over demos/auth.
setup_case() {
  unset TEST_MODE TEST_GENERATION
  case_dir="$test_root/$1 workspace"
  layout_dir="$case_dir/chi-auth-layout"
  sql_dir="$case_dir/demos/auth/internal/infra/db/migrations"
  auth_config="$case_dir/demos/auth/conf/auth.yml"
  mkdir -p "$layout_dir" "$sql_dir" "$(dirname "$auth_config")" "$case_dir/expected" "$case_dir/demos/auth/bin" "$case_dir/demos/another-demo" "$case_dir/demos/auth/internal/modules/whitelist"
  printf 'keep sibling\n' > "$case_dir/demos/another-demo/sentinel"
  printf 'obsolete source\n' > "$case_dir/demos/auth/internal/modules/whitelist/http.go"
  printf 'old local override\n' > "$case_dir/demos/auth/.env.local"
  printf 'original binary\n' > "$case_dir/demos/auth/bin/auth"
  cp "$template_dir/Makefile" "$layout_dir/Makefile"
  for suffix in 01-auth-schema 02-auth-default-data 03-action-default-data 04-dictionary-default-data; do
    printf 'original %s\n' "$suffix" > "$sql_dir/2026010101-$suffix.sql"
  done
  for suffix in 05-resource-permission-schema 11-sparse-default-groups 99-local-only; do
    printf 'historical %s\n' "$suffix" > "$sql_dir/2026030303-$suffix.sql"
  done
  cp -p "$sql_dir"/*.sql "$case_dir/expected/"
  printf 'keep non-sql files\n' > "$sql_dir/README.txt"
  cat > "$auth_config" <<'YAML'
auth:
  jwt:
    key: "original-test-key"
    issuer: old
  switches:
    enabled: true
YAML
  cp "$auth_config" "$case_dir/original-auth.yml"
  cat > "$layout_dir/fake-scaffold" <<'STUB'
#!/bin/sh
set -eu
test "$1" = new
output_dir=
for arg in "$@"; do
  case "$arg" in
    --output-dir=*) output_dir=${arg#--output-dir=} ;;
    --overwrite|--force) exit 9 ;;
  esac
done
test -n "$output_dir"
touch scaffold-called
sql_dir="$output_dir/auth/internal/infra/db/migrations"
test ! -e "$output_dir/auth"
if [ "${TEST_MODE:-success}" = early-failure ]; then exit 7; fi
mkdir -p "$sql_dir" "$output_dir/auth/conf" "$output_dir/auth/bin"
timestamp=2026101920
if [ "${TEST_MODE:-success}" = out-of-order ]; then timestamp=2025010101; fi
for suffix in 01-auth-schema 02-auth-default-data 03-action-default-data 04-dictionary-default-data 05-resource-permission-schema 11-sparse-default-groups 12-new-feature 13-another-feature; do
  printf 'generated %s %s\n' "${TEST_GENERATION:-1}" "$suffix" > "$sql_dir/$timestamp-$suffix.sql"
done
cat > "$output_dir/auth/conf/auth.yml" <<'YAML'
auth:
  jwt:
    key: "generated-test-key"
    issuer: generated
  switches:
    enabled: true
YAML
cat > "$output_dir/auth/Makefile" <<'MAKE'
.PHONY: build
build:
	@touch rebuilt
	@if [ "$${TEST_MODE:-success}" = build-failure ]; then exit 8; fi
	@cat internal/infra/db/migrations/*.sql > bin/auth
MAKE
cat "$sql_dir"/*.sql > "$output_dir/auth/bin/auth"
if [ "${TEST_MODE:-success}" = failure ]; then exit 7; fi
STUB
  chmod +x "$layout_dir/fake-scaffold"
}

run_local() {
  if (cd "$layout_dir" && make -s local SCAFFOLD=./fake-scaffold "$@") > "$case_dir/run.log" 2>&1; then
    return 0
  else
    result=$?
    cat "$case_dir/run.log" >&2
    return "$result"
  fi
}

assert_sql() {
  actual_dir="$case_dir/actual"
  mkdir -p "$actual_dir"
  rm -f "$actual_dir"/*.sql
  cp -p "$sql_dir"/*.sql "$actual_dir/"
  diff -ru "$case_dir/expected" "$actual_dir"
  test ! -e "$sql_dir/README.txt"
  test ! -e "$case_dir/demos/auth/internal/modules/whitelist"
  test ! -e "$case_dir/demos/auth/.env.local"
  test "$(cat "$case_dir/demos/another-demo/sentinel")" = 'keep sibling'
}

assert_jwt() {
  test "$(sed -n '/^    key:/p' "$auth_config")" = '    key: "original-test-key"'
  ! rg -q 'generated-test-key|original-test-key' "$case_dir/run.log"
}

assert_binary() {
  cat "$sql_dir"/*.sql > "$case_dir/expected-binary"
  cmp "$case_dir/expected-binary" "$case_dir/demos/auth/bin/auth"
}

setup_case preserve
run_local DELETE_OLD_SQL=false
for suffix in 12-new-feature 13-another-feature; do
  printf 'generated 1 %s\n' "$suffix" > "$case_dir/expected/2026101920-$suffix.sql"
done
assert_sql
assert_jwt
assert_binary
test -f "$case_dir/demos/auth/rebuilt"
rg -q 'issuer: generated' "$auth_config"
# Re-running in the same hour must not overwrite even newly added migrations.
TEST_GENERATION=2 run_local DELETE_OLD_SQL=false
assert_sql
assert_jwt
assert_binary
printf 'PASS preserve historical SQL and append new migrations; repeated generation is stable\n'

for setting in true default; do
  setup_case "replace-$setting"
  if [ "$setting" = default ]; then run_local; else run_local DELETE_OLD_SQL=true; fi
  rm "$case_dir/expected"/*.sql
  for suffix in 01-auth-schema 02-auth-default-data 03-action-default-data 04-dictionary-default-data 05-resource-permission-schema 11-sparse-default-groups 12-new-feature 13-another-feature; do
    printf 'generated 1 %s\n' "$suffix" > "$case_dir/expected/2026101920-$suffix.sql"
  done
  assert_sql
  assert_jwt
  assert_binary
  test ! -e "$case_dir/demos/auth/rebuilt"
  printf 'PASS replacement mode (%s)\n' "$setting"
done

for setting in true false; do
  setup_case "failure-$setting"
  if TEST_MODE=failure run_local "DELETE_OLD_SQL=$setting"; then
    printf 'expected scaffold failure to propagate\n' >&2
    exit 1
  fi
  assert_sql
  assert_jwt
  test "$(cat "$case_dir/demos/auth/bin/auth")" = 'original binary'
  printf 'PASS restore original SQL and JWT after generation failure (%s)\n' "$setting"
done

setup_case out-of-order
if TEST_MODE=out-of-order run_local DELETE_OLD_SQL=false; then
  printf 'expected out-of-order migration to be rejected\n' >&2
  exit 1
fi
rg -q 'must sort after existing migration' "$case_dir/run.log"
assert_sql
assert_jwt
test "$(cat "$case_dir/demos/auth/bin/auth")" = 'original binary'
printf 'PASS reject out-of-order additions and restore original SQL\n'

for prior_binary in present absent; do
  setup_case "build-failure-$prior_binary"
  if [ "$prior_binary" = absent ]; then rm "$case_dir/demos/auth/bin/auth"; fi
  if TEST_MODE=build-failure run_local DELETE_OLD_SQL=false; then
    printf 'expected final build failure to propagate\n' >&2
    exit 1
  fi
  assert_sql
  assert_jwt
  if [ "$prior_binary" = present ]; then
    test "$(cat "$case_dir/demos/auth/bin/auth")" = 'original binary'
  else
    test ! -e "$case_dir/demos/auth/bin/auth"
  fi
  printf 'PASS restore original SQL and binary after final build failure (%s)\n' "$prior_binary"
done

setup_case invalid
if run_local DELETE_OLD_SQL=invalid; then
  printf 'expected invalid flag to be rejected\n' >&2
  exit 1
fi
test ! -e "$layout_dir/scaffold-called"
rg -q 'DELETE_OLD_SQL must be true or false' "$case_dir/run.log"
diff -ru "$case_dir/expected" "$sql_dir" -x README.txt
test -f "$sql_dir/README.txt"
test -f "$case_dir/demos/auth/internal/modules/whitelist/http.go"
test -f "$case_dir/demos/auth/.env.local"
cmp "$auth_config" "$case_dir/original-auth.yml"
printf 'PASS reject invalid flag before changing files\n'

for setting in true false; do
  setup_case "early-failure-$setting"
  if TEST_MODE=early-failure run_local "DELETE_OLD_SQL=$setting"; then
    printf 'expected early generation failure to propagate\n' >&2
    exit 1
  fi
  assert_sql
  assert_jwt
  cmp "$auth_config" "$case_dir/original-auth.yml"
  test "$(cat "$case_dir/demos/auth/bin/auth")" = 'original binary'
  printf 'PASS restore SQL, configuration and binary after failure before generation (%s)\n' "$setting"
done

for setting in true false; do
  setup_case "absent-$setting"
  rm -rf "$case_dir/demos/auth"
  run_local "DELETE_OLD_SQL=$setting"
  test -f "$auth_config"
  rg -q 'generated-test-key' "$auth_config"
  test -f "$case_dir/demos/auth/bin/auth"
  assert_binary
  test "$(cat "$case_dir/demos/another-demo/sentinel")" = 'keep sibling'
  printf 'PASS generate an absent demo (%s)\n' "$setting"
done
