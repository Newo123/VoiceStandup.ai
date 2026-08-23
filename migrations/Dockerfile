FROM golang:1.25-alpine AS builder

ARG GOOSE_VERSION=v3.27.3

RUN CGO_ENABLED=0 go install \
    -tags="no_clickhouse no_libsql no_mssql no_mysql no_sqlite3 no_vertica no_ydb" \
    github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION}

FROM alpine:3.23

RUN apk add --no-cache ca-certificates

COPY --from=builder /go/bin/goose /usr/local/bin/goose

ENTRYPOINT ["goose"]
