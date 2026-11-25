# syntax=docker/dockerfile:1

# --- web UI ---------------------------------------------------------------
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Go binary --------------------------------------------------------------
FROM golang:1.27-alpine AS build
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/rootcausehq/rootcause/internal/version.Version=${VERSION} -X github.com/rootcausehq/rootcause/internal/version.Commit=${COMMIT}" \
      -o /out/rootcause ./cmd/rootcause

# --- runtime: no shell, no package manager, non-root ---------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/rootcause /usr/local/bin/rootcause
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/rootcause"]
CMD ["serve"]
