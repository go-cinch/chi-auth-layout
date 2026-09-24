APP := {{ .Computed.service_name_kebab }}
MODULE := $(shell go list -m)
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short=5 HEAD 2>/dev/null)
ifeq ($(strip $(VERSION)),)
VERSION := dev
endif
LDFLAGS := -s -w -X $(MODULE)/internal/common/config.Version=$(VERSION)

{{ if .Computed.enable_grpc_final -}}
GRPCUI_VERSION ?= v1.5.4
GRPCUI_BIN := $(CURDIR)/.tools/grpcui-$(GRPCUI_VERSION)/grpcui
GRPCUI ?= $(GRPCUI_BIN)
GRPC_ADDR ?= $(if $(SERVICE_GRPC_ADDR),$(SERVICE_GRPC_ADDR),$(shell awk '$$1 == "addr:" { gsub(/["\047]/, "", $$2); print $$2; exit }' conf/grpc.yml))
GRPCUI_PORT ?= 8090
GRPC_SERVICE ?= {{ .Computed.proto_service_name_final }}

{{ end -}}
.PHONY: config api gen tidy fmt lint test build run clean{{ if .Computed.enable_grpc_final }} proto grpcui debug-grpc{{ end }}

config:
	go run -mod=mod ./internal/cmd/configgen -config-dir ./conf -output ./internal/common/config/config.gen.go

{{ if .Computed.enable_grpc_final -}}
proto:
	go run -mod=mod ./internal/cmd/protogen

{{ end -}}
api:{{ if .Computed.enable_grpc_final }} proto{{ end }}
	go run ./internal/cmd/modulegen -modules-dir ./internal/modules -app-file ./internal/app/app.go -go-mod ./go.mod -output ./internal/app/modules.gen.go
	go run ./internal/cmd/docsgen -source ./internal -config-dir ./conf -output ./internal/docs/openapi.yaml -title "$(APP) API"

gen: config
	$(MAKE) api

tidy:
	go mod tidy

fmt:
	gofmt -w .

lint: gen
	go vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

test: gen
	go test -race -coverprofile=coverage.out ./...
	awk -f ./scripts/check-coverage.awk coverage.out

build: gen
	mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o ./bin/$(APP) ./cmd/server

run: gen
	go run ./cmd/server -c ./conf

{{ if .Computed.enable_grpc_final -}}
# Install the pinned project-local grpcui unless an executable is explicitly supplied.
grpcui:
	@if [ "$(GRPCUI)" = "$(GRPCUI_BIN)" ] && [ ! -x "$(GRPCUI_BIN)" ]; then \
		mkdir -p "$(dir $(GRPCUI_BIN))" && \
		GOBIN="$(dir $(GRPCUI_BIN))" go install -ldflags "-X main.version=$(GRPCUI_VERSION)" github.com/fullstorydev/grpcui/cmd/grpcui@$(GRPCUI_VERSION); \
	fi

# Open grpcui for the running service; use config/env unless GRPC_ADDR is set.
debug-grpc: grpcui
	@addr="$(GRPC_ADDR)"; \
	case "$$addr" in \
		:*) addr="127.0.0.1$$addr" ;; \
		0.0.0.0:*) addr="127.0.0.1:$${addr##*:}" ;; \
		"[::]:"*) addr="[::1]:$${addr##*:}" ;; \
	esac; \
	test -n "$$addr" || { echo "Set GRPC_ADDR or grpc.addr in conf/grpc.yml" >&2; exit 1; }; \
	set -- -plaintext -bind 127.0.0.1 -port "$(GRPCUI_PORT)"; \
	has_proto=false; \
	for proto in api/$(GRPC_SERVICE)-proto/*.proto; do \
		[ -f "$$proto" ] || continue; \
		set -- "$$@" -proto "$$proto"; has_proto=true; \
	done; \
	if [ "$$has_proto" = true ]; then \
		set -- "$$@" -import-path . -use-reflection=false; \
		[ ! -d third_party ] || set -- "$$@" -import-path third_party; \
	else \
		set -- "$$@" -use-reflection; \
	fi; \
	exec "$(GRPCUI)" "$$@" "$$addr"

{{ end -}}
clean:
	rm -rf ./bin ./coverage.out
