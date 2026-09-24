module {{ .Computed.module_name_final }}

go 1.27.1

require (
	golang.org/x/text v0.41.0
{{- if .Computed.enable_grpc_final }}
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.12
{{- end }}
{{- if eq .Computed.http_router_final "gin" }}
	github.com/gin-gonic/gin v1.12.0
{{- else }}
	github.com/go-chi/chi/v5 v5.3.2
{{- end }}
	github.com/knadh/koanf/parsers/yaml v1.1.0
	github.com/knadh/koanf/providers/file v1.2.1
	github.com/knadh/koanf/v2 v2.3.5
	github.com/pmezard/go-difflib v1.0.0
	github.com/twpayne/go-jsonstruct/v3 v3.3.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/go-jose/go-jose/v4 v4.1.5
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/lib/pq v1.12.3
{{- if .Computed.enable_trace_final }}
	github.com/XSAM/otelsql v0.42.0
{{- end }}
	github.com/rubenv/sql-migrate v1.8.1
{{- if .Computed.enable_redis_final }}
	github.com/redis/go-redis/v9 v9.21.0
{{- end }}
{{- if .Computed.enable_trace_final }}
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
{{- end }}
	golang.org/x/crypto v0.49.0
)
