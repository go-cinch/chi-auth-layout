.PHONY: lint test test-local full local

SCAFFOLD ?= scaffold
PROJECT ?= auth-service
OUTPUT_DIR ?= ..
HTTP_ROUTER ?= chi
ENABLE_GRPC ?= true
DELETE_OLD_SQL ?= true

ifneq ($(ENABLE_GRPC),true)
ifneq ($(ENABLE_GRPC),false)
$(error ENABLE_GRPC must be true or false)
endif
endif

ifneq ($(HTTP_ROUTER),chi)
ifneq ($(HTTP_ROUTER),gin)
$(error HTTP_ROUTER must be chi or gin)
endif
endif

lint:
	$(SCAFFOLD) lint scaffold.yml

test: lint test-local
	./scripts/test-template.sh

test-local:
	sh ./scripts/test-local.sh

full:
	$(SCAFFOLD) new "$(CURDIR)" --output-dir="$(OUTPUT_DIR)" --run-hooks=always --no-prompt --preset=full "Project=$(PROJECT)" "http_router=$(HTTP_ROUTER)" "enable_grpc=$(ENABLE_GRPC)"

# Use DELETE_OLD_SQL=false when retaining an initialized local database.
# Generate SQL separately from the immutable backup. In preserve mode, match
# regenerated migrations by NN-description and append only new migrations.
local:
	@set -eu; \
	delete_old_sql='$(DELETE_OLD_SQL)'; \
	case "$$delete_old_sql" in true|false) ;; *) printf 'DELETE_OLD_SQL must be true or false\n' >&2; exit 2 ;; esac; \
	migrations_dir="$(CURDIR)/../demos/auth/internal/infra/db/migrations"; \
	auth_config="$(CURDIR)/../demos/auth/conf/auth.yml"; \
	app_binary="$(CURDIR)/../demos/auth/bin/auth"; \
	test -d "$$migrations_dir"; \
	test -f "$$auth_config"; \
	if [ "$$delete_old_sql" = false ]; then \
		for suffix in 01-auth-schema 02-auth-default-data; do \
			count=$$(find "$$migrations_dir" -maxdepth 1 -type f -name "*-$$suffix.sql" | wc -l | tr -d ' '); \
			if [ "$$count" -ne 1 ]; then \
				printf 'expected exactly one *-%s.sql in %s, found %s\n' "$$suffix" "$$migrations_dir" "$$count" >&2; \
				exit 1; \
			fi; \
		done; \
	fi; \
	backup_dir=$$(mktemp -d "$${TMPDIR:-/tmp}/chi-auth-layout-state.XXXXXX"); \
	mkdir "$$backup_dir/sql"; \
	if [ -f "$$app_binary" ]; then cp -p "$$app_binary" "$$backup_dir/original-binary"; fi; \
	awk ' \
		$$0 == "  jwt:" { in_jwt = 1; next } \
		in_jwt && /^  [^ ]/ { in_jwt = 0 } \
		in_jwt && /^    key:/ { print; matches++ } \
		END { if (matches != 1) exit 1 } \
	' "$$auth_config" > "$$backup_dir/jwt-key-line"; \
	find "$$migrations_dir" -maxdepth 1 -type f -name '*.sql' \
		-exec cp -p {} "$$backup_dir/sql/" \;; \
	restore_state() { \
		status=$$?; \
		trap - EXIT INT TERM; \
		if [ "$$delete_old_sql" = false ] || [ "$$status" -ne 0 ]; then \
			if [ "$$status" -eq 0 ]; then \
				mkdir "$$backup_dir/new-sql"; \
				latest=$$(find "$$backup_dir/sql" -maxdepth 1 -type f -name '*.sql' -exec basename {} \; | LC_ALL=C sort | tail -n 1); \
				for migration in "$$migrations_dir"/*.sql; do \
					[ -f "$$migration" ] || continue; \
					name=$${migration##*/}; \
					suffix=$${name#*-}; \
					known=false; \
					for previous in "$$backup_dir/sql"/*.sql; do \
						[ -f "$$previous" ] || continue; \
						previous_name=$${previous##*/}; \
						if [ "$${previous_name#*-}" = "$$suffix" ]; then known=true; break; fi; \
					done; \
					[ "$$known" = false ] || continue; \
					first=$$(printf '%s\n%s\n' "$$latest" "$$name" | LC_ALL=C sort | head -n 1); \
					if [ -n "$$latest" ] && [ "$$first" != "$$latest" ]; then \
						printf 'new migration %s must sort after existing migration %s\n' "$$name" "$$latest" >&2; \
						status=1; break; \
					fi; \
					if ! cp -p "$$migration" "$$backup_dir/new-sql/"; then status=1; break; fi; \
				done; \
			fi; \
			find "$$migrations_dir" -maxdepth 1 -type f -name '*.sql' -exec rm -f {} +; \
			find "$$backup_dir/sql" -maxdepth 1 -type f -name '*.sql' -exec cp -p {} "$$migrations_dir/" \;; \
			if [ "$$status" -eq 0 ]; then \
				find "$$backup_dir/new-sql" -maxdepth 1 -type f -name '*.sql' -exec cp -p {} "$$migrations_dir/" \;; \
			fi; \
		fi; \
		awk -v replacement_file="$$backup_dir/jwt-key-line" ' \
			BEGIN { \
				if ((getline replacement < replacement_file) <= 0) exit 1; \
				close(replacement_file); \
			} \
			$$0 == "  jwt:" { in_jwt = 1; print; next } \
			in_jwt && /^  [^ ]/ { in_jwt = 0 } \
			in_jwt && /^    key:/ { print replacement; replaced++; next } \
			{ print } \
			END { if (replaced != 1) exit 1 } \
		' "$$auth_config" > "$$backup_dir/auth.yml"; \
		mv "$$backup_dir/auth.yml" "$$auth_config"; \
		if [ "$$status" -eq 0 ] && [ "$$delete_old_sql" = false ]; then \
			if make -C "$(CURDIR)/../demos/auth" build; then :; else \
				status=$$?; \
				find "$$migrations_dir" -maxdepth 1 -type f -name '*.sql' -exec rm -f {} +; \
				find "$$backup_dir/sql" -maxdepth 1 -type f -name '*.sql' -exec cp -p {} "$$migrations_dir/" \;; \
			fi; \
		fi; \
		if [ "$$status" -ne 0 ]; then \
			if [ -f "$$backup_dir/original-binary" ]; then \
				cp -p "$$backup_dir/original-binary" "$$app_binary"; \
			else rm -f "$$app_binary"; fi; \
		fi; \
		rm -r "$$backup_dir"; \
		exit "$$status"; \
	}; \
	trap restore_state EXIT; \
	trap 'exit 130' INT; \
	trap 'exit 143' TERM; \
	find "$$migrations_dir" -maxdepth 1 -type f -name '*.sql' -exec rm -f {} +; \
	$(SCAFFOLD) new "$(CURDIR)" --output-dir="$(CURDIR)/../demos" --run-hooks=always --no-prompt --preset=full --overwrite --force "Project=auth" "http_port=8081" "http_router=$(HTTP_ROUTER)" "enable_grpc=$(ENABLE_GRPC)"
