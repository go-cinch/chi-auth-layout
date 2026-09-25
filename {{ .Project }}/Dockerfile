FROM golang:1.27.1-alpine3.23 AS builder

ARG VERSION

ENV CGO_ENABLED=0

RUN apk add --no-cache ca-certificates git make{{ if .Computed.enable_grpc_final }} protobuf{{ end }}

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN if [ -n "$VERSION" ]; then \
        make build VERSION="$VERSION"; \
    else \
        make build; \
    fi

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app -u 10001 app

WORKDIR /app

COPY --from=builder /src/bin/{{ .Computed.service_name_kebab }} /app/{{ .Computed.service_name_kebab }}
COPY conf /app/conf
COPY POINT_CAPTCHA_FONT_LICENSE.txt /app/POINT_CAPTCHA_FONT_LICENSE.txt

USER app

ENTRYPOINT ["/app/{{ .Computed.service_name_kebab }}"]
CMD ["-c", "/app/conf"]
