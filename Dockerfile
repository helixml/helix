### API Base ###
#---------------
# Debian is required for CGo (hugot tokenizers link against glibc)
# Pin to specific digest for stable layer caching.
# Update digest when intentionally upgrading Go version.
FROM golang:1.25-bookworm@sha256:e3a54b77385b4f8a31c1db4d12429ffb3718ea76865731a787c497755d409547 AS api-base
WORKDIR /app
# Install build dependencies for CGo (hugot/tokenizers)
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential git \
    && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
# helix-org is a replace target (`replace github.com/helixml/helix-org => ./helix-org`),
# so `go mod download` needs at least its go.mod/go.sum present to resolve the module.
COPY helix-org/go.mod helix-org/go.sum ./helix-org/
# Cache Go modules for offline builds
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

### Embedding model stage ###
#----------------------------
# Downloads and converts the st-codesearch-distilroberta-base model to ONNX format.
# Uses kodit's download-model tool (Go binary that embeds the Python conversion script).
FROM api-base AS embedding-model
COPY --from=ghcr.io/astral-sh/uv:debian-slim /usr/local/bin/uv /usr/local/bin/uv
# Cache the uv wheel downloads (~2GB of torch/transformers/cuda-* wheels) and
# the HuggingFace model snapshot so re-running these stages on the same builder
# doesn't re-download from PyPI / HF Hub each time.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/.cache/uv \
    --mount=type=cache,target=/root/.cache/huggingface \
    go run github.com/helixml/kodit/cmd/download-model /build/models/flax-sentence-embeddings_st-codesearch-distilroberta-base
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/.cache/uv \
    --mount=type=cache,target=/root/.cache/huggingface \
    go run github.com/helixml/kodit/cmd/download-siglip2 /build/models/google_siglip2-base-patch16-512

### Tokenizers library ###
#-------------------------
# Downloads libtokenizers.a via kodit's download-ort tool
FROM api-base AS tokenizers-lib
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go run github.com/helixml/kodit/tools/download-ort

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
COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/
# Tell kodit where to find the ORT library (see production stage comment for details)
ENV ORT_LIB_DIR=/usr/lib
# - Copy embedding models for kodit code intelligence
COPY --from=embedding-model /build/models/ /kodit-models/
# - Copy the files and run a build to make startup faster
COPY api /app/api
# helix-org sources for the replace directive — dev mode bind-mounts over
# this layer at runtime, but the initial pre-build needs the real sources.
COPY helix-org /app/helix-org
WORKDIR /app/api
# - Run a build to make the initial air build faster
# Cache Go modules and build artifacts for offline builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -tags ORT -ldflags "-s -w" -o /helix
# - Entrypoint is the air command
ENTRYPOINT ["air", "--build.bin", "/helix", "--build.cmd", "CGO_ENABLED=1 go build -tags ORT -ldflags \"-s -w\" -o /helix", "--build.stop_on_error", "true", "--"]
CMD ["serve"]


#### API Build ###
#-----------------------
FROM api-base AS api-build-env
# Following git lines required for buildvcs to work
COPY .git /app/.git
# Copy tokenizers library for CGo
COPY --from=tokenizers-lib /app/lib/libtokenizers.a /usr/lib/
COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/
COPY api /app/api
# helix-org sources for the replace directive in the root go.mod.
COPY helix-org /app/helix-org
WORKDIR /app/api
# - main.version is a variable required by Sentry and is set in .drone.yaml
ARG APP_VERSION="v0.0.0+unknown"
# Cache Go modules and build artifacts for offline builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -tags ORT -buildvcs=true -ldflags "-s -w -X main.version=$APP_VERSION -X github.com/helixml/helix/api/pkg/data.Version=$APP_VERSION" -o /helix

### Frontend Base ###
#--------------------
# Pin to specific digest for stable layer caching.
# Update digest when intentionally upgrading Node version.
FROM node:23-alpine@sha256:a34e14ef1df25b58258956049ab5a71ea7f0d498e41d0b514f4b8de09af09456 AS ui-base
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
# Pin to specific digest for stable layer caching.
# Update digest when intentionally upgrading Debian version.
FROM debian:bookworm-slim@sha256:67b30a61dc87758f0caf819646104f29ecbda97d920aaf5edc834128ac8493d3
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git git-daemon-sysvinit \
    && rm -rf /var/lib/apt/lists/*

COPY --from=api-build-env /helix /helix
COPY --from=ui-build-env /app/dist /www
# Embedding model files for kodit code intelligence
COPY --from=embedding-model /build/models/ /kodit-models/
# ONNX Runtime library required by kodit's Hugot embedding provider (built with -tags ORT)
COPY --from=tokenizers-lib /app/lib/libonnxruntime.so /usr/lib/
# Tell kodit where to find the ORT library. Without this, kodit's auto-detection
# resolves to /lib (because the binary is at /helix → filepath.Dir = /) which
# may fail depending on usrmerge symlink state and library availability.
ENV ORT_LIB_DIR=/usr/lib

ENV FRONTEND_URL=/www

EXPOSE 80

ENTRYPOINT ["/helix", "serve"]
