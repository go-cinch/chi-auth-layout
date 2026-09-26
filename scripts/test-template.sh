#!/bin/sh

set -eu

template_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/chi-layout-test.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

validate_project() (
  generated=$1
  router=$2
  grpc=${3:-true}
  cd "$generated"
  test -f .dockerignore
  ! rg -q '^/?\.git/?$' .dockerignore
  rg -q '^ARG VERSION$' Dockerfile
  rg -q '^RUN make build VERSION="\$VERSION"$' Dockerfile
  ! rg -q 'go build.*ldflags' Dockerfile
  make gen
  cp internal/docs/openapi.yaml "$tmp_dir/expected-openapi.yaml"
  make gen
  cmp internal/docs/openapi.yaml "$tmp_dir/expected-openapi.yaml"
  gofmt -w .
  go mod tidy
  make lint test build
  go list -m all > "$tmp_dir/dependencies.txt"
  go list -deps ./cmd/server > "$tmp_dir/server-dependencies.txt"
  test ! -e internal/modules/auth/POINT_CAPTCHA_FONT_LICENSE.txt
  test -f conf/auth.yml
  test -f conf/database.yml
  rg -q '^  driver: postgres$' conf/database.yml
  rg -q '^  migrate: true$' conf/database.yml
  test -f internal/infra/db/migrations_embed.go
  test -f internal/infra/db/migrate.go
  for module in auth user role action usergroup whitelist dictionary; do
    test -f "internal/modules/$module/http.go"
  done
  test -f internal/common/authn/authn.go
  test -f internal/common/idempotency/store.go
  for suffix in 01-auth-schema 02-auth-default-data; do
    test "$(find internal/infra/db/migrations -maxdepth 1 -name "*-$suffix.sql" | wc -l | tr -d ' ')" = 1
  done
  rg -q '/auth/pub/login' internal/docs/openapi.yaml
  rg -q '/auth/pub/register' internal/docs/openapi.yaml
  rg -q '/auth/pub/challenge' internal/docs/openapi.yaml
  rg -q '/auth/pub/captcha' internal/docs/openapi.yaml
  rg -q '^  /auth/reset/pwd:' internal/docs/openapi.yaml
  rg -q '^  /auth/challenge:' internal/docs/openapi.yaml
  rg -q '^  /auth/captcha:' internal/docs/openapi.yaml
  rg -q '/auth/pub/register/username' internal/docs/openapi.yaml
  rg -q '/user/\{id\}' internal/docs/openapi.yaml
  ! rg -q '^  /auth/codes:' internal/docs/openapi.yaml
  ! rg -q '/auth/pub/login/challenge|/auth/pub/register/challenge|/auth/change/pwd/challenge|/auth/pub/login/captcha|/auth/change/pwd/captcha' internal/docs/openapi.yaml
  test -f POINT_CAPTCHA_FONT_LICENSE.txt
  rg -q 'Noto Sans CJK SC Medium' POINT_CAPTCHA_FONT_LICENSE.txt
  rg -q '^COPY POINT_CAPTCHA_FONT_LICENSE.txt /app/POINT_CAPTCHA_FONT_LICENSE.txt$' Dockerfile
  rg -q '^  challengeEncryption:$' conf/auth.yml
  rg -q '^  switches:$' conf/auth.yml
  rg -q '^    passwordResetRequired: true$' conf/auth.yml
  rg -q '^    protectSuper: false$' conf/auth.yml
  rg -q '^    protectCaptchaDictionaries: false$' conf/auth.yml
  rg -q '^    ttl: 1m$' conf/auth.yml
  ! rg -q 'loginEncryption|challengeTTL' conf/auth.yml
  rg -q 'ChallengeEncryption AuthChallengeEncryptionConfig `koanf:"challengeEncryption"`' internal/common/config/config.gen.go
  rg -q 'TTL[[:space:]]+time.Duration[[:space:]]+`koanf:"ttl"`' internal/common/config/config.gen.go
  jwt_key=$(awk '$1 == "key:" { gsub(/"/, "", $2); print $2; exit }' conf/auth.yml)
  test "${#jwt_key}" -eq 64
  case "$jwt_key" in
    *[![:alnum:]]*)
      echo "generated JWT key contains unexpected characters" >&2
      exit 1
      ;;
  esac
  test "$jwt_key" != development-only-jwt-signing-key-change-me
  if [ "$grpc" = false ]; then
    test ! -e conf/grpc.yml
    test ! -e internal/common/rpc
    test ! -e internal/cmd/protogen
    test ! -e api
    test ! -e third_party
    test ! -e .tools
    if rg -ni 'grpc|protobuf|protogen' internal/app internal/modules internal/cmd AGENTS.md Makefile Dockerfile README.md; then
      echo "gRPC references remain in an HTTP-only project" >&2
      exit 1
    fi
    if [ ! -e conf/tracer.yml ]; then
      if rg -q '^google.golang.org/grpc(/|$)' "$tmp_dir/server-dependencies.txt"; then
        echo "gRPC dependency remains with gRPC and tracing disabled" >&2
        exit 1
      fi
      # Check compiled packages: PostgreSQL migration dependencies can retain
      # unused protobuf module metadata. Gin can use protobuf for HTTP binding.
      if [ "$router" = chi ] && rg -q '^google.golang.org/protobuf(/|$)' "$tmp_dir/server-dependencies.txt"; then
        echo "protobuf dependency remains in the minimal chi project" >&2
        exit 1
      fi
    fi
  else
    test -f conf/grpc.yml
    test -f internal/common/rpc/server.go
    test -f internal/cmd/protogen/main.go
  fi
  if [ "$router" = gin ]; then
    rg -q '^github.com/gin-gonic/gin ' "$tmp_dir/dependencies.txt"
    ! rg -q '^github.com/go-chi/chi' "$tmp_dir/dependencies.txt"
    test ! -f internal/common/server/writer.go
  else
    rg -q '^github.com/go-chi/chi/v5 ' "$tmp_dir/dependencies.txt"
    ! rg -q '^github.com/gin-gonic/gin ' "$tmp_dir/dependencies.txt"
  fi
)

for router in chi gin; do
  for variant in full minimal no-preset; do
    project="$router-$variant"
    output_dir="$tmp_dir/$project"
    mkdir -p "$output_dir"
    printf 'Generating %s (%s)...\n' "$router" "$variant"
    set --
    if [ "$variant" != no-preset ]; then set -- --preset=full; fi
    grpc=true
    if [ "$variant" = minimal ]; then
      grpc=false
      set -- "$@" enable_grpc=false enable_trace=false enable_redis=false enable_health_check=false
    fi
    # Omitting chi exercises the router default.
    if [ "$router" = gin ]; then set -- "$@" http_router=gin; fi
    scaffold new "$template_dir" \
      --output-dir="$output_dir" --run-hooks=never --no-prompt \
      "Project=$project" "module_name=example.com/$project" "$@"
    validate_project "$output_dir/$project" "$router" "$grpc"
    if [ "$variant" = minimal ]; then
      test ! -e "$output_dir/$project/conf/redis.yml"
      test ! -e "$output_dir/$project/conf/tracer.yml"
      test ! -e "$output_dir/$project/internal/common/server/health.go"
    fi
  done

  # Exercise the public Make entry point and post-scaffold hook for each router.
  project="$router-hook"
  output_dir="$tmp_dir/$project"
  mkdir -p "$output_dir"
  printf 'Generating %s through Make with hooks...\n' "$router"
  grpc=true
  if [ "$router" = gin ]; then grpc=false; fi
  make -C "$template_dir" full "PROJECT=$project" "OUTPUT_DIR=$output_dir" "HTTP_ROUTER=$router" "ENABLE_GRPC=$grpc"
  test -f "$output_dir/$project/internal/common/config/config.gen.go"
  test -f "$output_dir/$project/internal/app/modules.gen.go"
  test -x "$output_dir/$project/bin/$project"
  test ! -e "$output_dir/$project/internal/cmd/seedcode"
  seed_files=$(find "$output_dir/$project/internal/infra/db/migrations" -maxdepth 1 -name '*-02-auth-default-data.sql')
  seed_codes=$(rg -o '\b[23456789ABCDEFGHJKLMNPQRSTVWXY]{8}\b' $seed_files | sed 's/^.*://' | sort -u)
  test "$(printf '%s\n' "$seed_codes" | wc -l | tr -d ' ')" = 32
  if rg -q 'SN2837AY|KHXK5JVL|2QKHTYEE|89HEK28Y|D1CTREAD|D1CTCRTE|D1CTUPDT|D1CTDELE' $seed_files; then
    echo "default seed codes were not randomized" >&2
    exit 1
  fi
  if [ "$router" = chi ]; then
    chi_hook_seed_codes=$seed_codes
  else
    gin_hook_seed_codes=$seed_codes
  fi
done

printf 'Checking JWT keys differ between generated projects...\n'
chi_jwt_key=$(awk '$1 == "key:" { gsub(/"/, "", $2); print $2; exit }' "$tmp_dir/chi-full/chi-full/conf/auth.yml")
gin_jwt_key=$(awk '$1 == "key:" { gsub(/"/, "", $2); print $2; exit }' "$tmp_dir/gin-full/gin-full/conf/auth.yml")
test "$chi_jwt_key" != "$gin_jwt_key"

printf 'Checking default seed codes differ between generated projects...\n'
test "$chi_hook_seed_codes" != "$gin_hook_seed_codes"

printf 'Checking invalid router values...\n'
if make -C "$template_dir" full HTTP_ROUTER=invalid "OUTPUT_DIR=$tmp_dir/invalid-make" > "$tmp_dir/invalid-make.log" 2>&1; then
  cat "$tmp_dir/invalid-make.log"
  exit 1
fi
rg -q 'HTTP_ROUTER must be chi or gin' "$tmp_dir/invalid-make.log"
if scaffold new "$template_dir" --output-dir="$tmp_dir/invalid-scaffold" \
  --run-hooks=never --no-prompt --preset=full Project=invalid http_router=invalid > "$tmp_dir/invalid-scaffold.log" 2>&1; then
  cat "$tmp_dir/invalid-scaffold.log"
  exit 1
fi
rg -q 'http_router must be chi or gin' "$tmp_dir/invalid-scaffold.log"
test ! -e "$tmp_dir/invalid-scaffold/invalid/go.mod"

printf 'Checking invalid gRPC selector values...\n'
if make -C "$template_dir" full ENABLE_GRPC=invalid "OUTPUT_DIR=$tmp_dir/invalid-grpc-make" > "$tmp_dir/invalid-grpc-make.log" 2>&1; then
  cat "$tmp_dir/invalid-grpc-make.log"
  exit 1
fi
rg -q 'ENABLE_GRPC must be true or false' "$tmp_dir/invalid-grpc-make.log"
if scaffold new "$template_dir" --output-dir="$tmp_dir/invalid-grpc-scaffold" \
  --run-hooks=never --no-prompt --preset=full Project=invalid enable_grpc=invalid > "$tmp_dir/invalid-grpc-scaffold.log" 2>&1; then
  cat "$tmp_dir/invalid-grpc-scaffold.log"
  exit 1
fi
rg -q 'enable_grpc must be true or false' "$tmp_dir/invalid-grpc-scaffold.log"
test ! -e "$tmp_dir/invalid-grpc-scaffold/invalid/go.mod"

printf 'All required-auth, PostgreSQL, router, optional-feature and hook checks passed.\n'
