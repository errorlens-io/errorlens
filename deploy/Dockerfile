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
      -ldflags "-s -w -X github.com/errorlens-io/errorlens/internal/version.Version=${VERSION} -X github.com/errorlens-io/errorlens/internal/version.Commit=${COMMIT}" \
      -o /out/errorlens ./cmd/errorlens

# --- runtime: no shell, no package manager, non-root ---------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/errorlens /usr/local/bin/errorlens
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/errorlens"]
CMD ["serve"]
