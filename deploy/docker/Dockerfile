# syntax=docker/dockerfile:1.7
# AgriSmart API — production image (Docker target). Build from the repo root:
#   docker build -f deploy/docker/Dockerfile -t agrismart-api .
# Behind a TLS-intercepting proxy, pass its CA: --secret id=ca,src=/path/ca.crt

FROM golang:1.26-alpine AS build
WORKDIR /src/backend
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=secret,id=ca,required=false \
    if [ -f /run/secrets/ca ]; then export SSL_CERT_FILE=/run/secrets/ca; fi; go mod download
COPY backend/ ./
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/api ./cmd/api && \
    go build -ldflags="-s -w" -o /out/migrate ./cmd/migrate && \
    go build -ldflags="-s -w" -o /out/seed ./cmd/seed

# distroless: no shell, no package manager, runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /usr/local/bin/
USER nonroot:nonroot
EXPOSE 8080
ENV AGRI_HTTP_ADDR=:8080 \
    AGRI_STORAGE_LOCAL_DIR=/tmp/media \
    AGRI_DATABASE_POOL_MODE=session
ENTRYPOINT ["/usr/local/bin/api"]
