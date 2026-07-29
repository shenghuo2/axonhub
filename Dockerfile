ARG VERSION
ARG SOURCE_REPOSITORY=https://github.com/looplj/axonhub

FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend-builder

WORKDIR /build
RUN corepack enable && corepack prepare pnpm@10 --activate
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

COPY ./frontend .
ENV NODE_OPTIONS="--max-old-space-size=4096"
RUN pnpm build

# Copy dist to a stage with the target platform to avoid architecture mismatch
FROM alpine AS frontend-dist
COPY --from=frontend-builder /build/dist /dist

FROM golang:alpine AS backend-builder

ARG VERSION

WORKDIR /build

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
COPY llm/go.mod llm/go.sum llm/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOTOOLCHAIN=auto go mod download

COPY . .
COPY --from=frontend-dist /dist /build/internal/server/static/dist

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    VERSION_VALUE="${VERSION:-$(cat internal/build/VERSION 2>/dev/null || echo dev)}" && \
    BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" && \
    GOTOOLCHAIN=auto go build \
      -tags=nomsgpack \
      -ldflags="-s -w -X github.com/looplj/axonhub/internal/build.Version=${VERSION_VALUE} -X github.com/looplj/axonhub/internal/build.BuildTime=${BUILD_TIME}" \
      -o axonhub \
      ./cmd/axonhub

FROM alpine

ARG VERSION
ARG SOURCE_REPOSITORY

LABEL org.opencontainers.image.title="AxonHub" \
      org.opencontainers.image.source="${SOURCE_REPOSITORY}" \
      org.opencontainers.image.version="${VERSION}"

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=backend-builder /build/axonhub /app/axonhub

EXPOSE 8090
ENTRYPOINT ["/app/axonhub"]
