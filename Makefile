.PHONY: lint test full local

SCAFFOLD ?= scaffold
PROJECT ?= auth-service
OUTPUT_DIR ?= ..
HTTP_ROUTER ?= chi
ENABLE_GRPC ?= true

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

test: lint
	./scripts/test-template.sh

full:
	$(SCAFFOLD) new "$(CURDIR)" --output-dir="$(OUTPUT_DIR)" --run-hooks=always --no-prompt --preset=full "Project=$(PROJECT)" "http_router=$(HTTP_ROUTER)" "enable_grpc=$(ENABLE_GRPC)"

# Set delete_old_sql to true below when this update must replace the demo SQL.
local:
	@set -eu; \
	delete_old_sql=true; \
	migrations_dir="$(CURDIR)/../demos/auth/internal/infra/db/migrations"; \
	auth_config="$(CURDIR)/../demos/auth/conf/auth.yml"; \
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
			find "$$migrations_dir" -maxdepth 1 -type f -name '*.sql' -exec rm -f {} +; \
			find "$$backup_dir/sql" -maxdepth 1 -type f -name '*.sql' -exec cp -p {} "$$migrations_dir/" \;; \
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
		rm -r "$$backup_dir"; \
		exit "$$status"; \
	}; \
	trap restore_state EXIT; \
	trap 'exit 130' INT; \
	trap 'exit 143' TERM; \
	if [ "$$delete_old_sql" = true ]; then \
		find "$$migrations_dir" -maxdepth 1 -type f -name '*.sql' -exec rm -f {} +; \
	else \
		find "$$migrations_dir" -maxdepth 1 -type f \( -name '*-01-auth-schema.sql' -o -name '*-02-auth-default-data.sql' \) -exec rm -f {} +; \
	fi; \
	$(SCAFFOLD) new "$(CURDIR)" --output-dir="$(CURDIR)/../demos" --run-hooks=always --no-prompt --preset=full --overwrite --force "Project=auth" "http_port=8081" "http_router=$(HTTP_ROUTER)" "enable_grpc=$(ENABLE_GRPC)"
