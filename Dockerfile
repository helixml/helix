### API Base ###
#---------------
# Debian is required for CGo (hugot tokenizers link against glibc)
FROM golang:1.25-bookworm AS api-base
WORKDIR /app
# Install build dependencies for CGo (hugot/tokenizers)
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential git \
    && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
# Cache Go modules for offline builds
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

### Embedding model stage ###
#----------------------------
# Downloads and converts the st-codesearch-distilroberta-base model to ONNX format.
# Uses kodit's download-model tool (Go binary that embeds the Python conversion script).
FROM api-base AS embedding-model
COPY --from=ghcr.io/astral-sh/uv:debian-slim@sha256:b852203fd7831954c58bfa1fec1166295adcfcfa50f4de7fdd0e684c8bd784eb /usr/local/bin/uv /usr/local/bin/uv
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go run github.com/helixml/kodit/cmd/download-model /build/models/flax-sentence-embeddings_st-codesearch-distilroberta-base

### Tokenizers library ###
#-------------------------
# Downloads libtokenizers.a via kodit's download-ort tool
FROM api-base AS tokenizers-lib
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    ORT_VERSION=1.24.1 go run github.com/helixml/kodit/tools/download-ort

### API Development ###
#----------------------
FROM api-base AS api-dev-env
# - Air provides hot reload for Go
RUN go install github.com/air-verse/air@v1.52.3
# - Install curl for Wolf API debugging, bash for git operations, and git-daemon for git-http-backend
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl bash git-daemon-sysvinit \
    && rm -rf /var/lib/apt/lists/*
# - Copy tokenizers library for CGo
COPY --from=tokenizers-lib /app/lib/libtokenizers.a /usr/lib/
# - Copy the files and run a build to make startup faster
COPY api /app/api
WORKDIR /app/api
# - Run a build to make the initial air build faster
# Cache Go modules and build artifacts for offline builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -ldflags "-s -w" -o /helix
# - Entrypoint is the air command
ENTRYPOINT ["air", "--build.bin", "/helix", "--build.cmd", "CGO_ENABLED=1 go build -ldflags \"-s -w\" -o /helix", "--build.stop_on_error", "true", "--"]
CMD ["serve"]


#### API Build ###
#-----------------------
FROM api-base AS api-build-env
# Following git lines required for buildvcs to work
COPY .git /app/.git
# Copy tokenizers library for CGo
COPY --from=tokenizers-lib /app/lib/libtokenizers.a /usr/lib/
COPY api /app/api
WORKDIR /app/api
# - main.version is a variable required by Sentry and is set in .drone.yaml
ARG APP_VERSION="v0.0.0+unknown"
# Cache Go modules and build artifacts for offline builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -buildvcs=true -ldflags "-s -w -X main.version=$APP_VERSION -X github.com/helixml/helix/api/pkg/data.Version=$APP_VERSION" -o /helix

### Frontend Base ###
#--------------------
FROM node:25-alpine AS ui-base
WORKDIR /app
# - Install dependencies
COPY ./frontend/*.json /app/
COPY ./frontend/yarn.lock /app/yarn.lock
# Cache yarn packages for offline builds
RUN --mount=type=cache,target=/usr/local/share/.cache/yarn \
    yarn install


### Frontend Development ###
#---------------------------
FROM ui-base AS ui-dev-env
ENTRYPOINT ["yarn"]
CMD ["run", "dev"]


### Frontend Build ###
#----------------------
FROM ui-base AS ui-build-env
# Copy the rest of the code
COPY ./frontend /app
# Build the frontend
RUN yarn build


### Production Image ###
#-----------------------
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git git-daemon-sysvinit \
    && rm -rf /var/lib/apt/lists/*

COPY --from=api-build-env /helix /helix
COPY --from=ui-build-env /app/dist /www
# Embedding model files for kodit code intelligence
COPY --from=embedding-model /build/models/ /kodit-models/

ENV FRONTEND_URL=/www

EXPOSE 80

ENTRYPOINT ["/helix", "serve"]
