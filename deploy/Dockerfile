# syntax=docker/dockerfile:1

# --- web UI ---------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Go binary --------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/errorlenshq/errorlens/internal/version.Version=${VERSION} -X github.com/errorlenshq/errorlens/internal/version.Commit=${COMMIT}" \
      -o /out/errorlens ./cmd/errorlens

# --- runtime: no shell, no package manager, non-root ---------------------------
FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="ErrorLens" \
      org.opencontainers.image.description="Open-source error tracking, performance monitoring, logs, cron and uptime monitoring" \
      org.opencontainers.image.url="https://errorlens.io" \
      org.opencontainers.image.documentation="https://errorlens.io/docs/self-hosting/" \
      org.opencontainers.image.source="https://github.com/errorlenshq/errorlens" \
      org.opencontainers.image.vendor="ErrorLens" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
COPY --from=build /out/errorlens /usr/local/bin/errorlens
USER nonroot:nonroot
EXPOSE 8080
# The binary checks its own /healthz (no shell or curl in this image). Disable it for --role worker containers,
# which serve no HTTP: healthcheck: { disable: true } in Compose.
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 CMD ["/usr/local/bin/errorlens", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/errorlens"]
CMD ["serve"]
