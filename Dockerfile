# Backend build
FROM golang:1.20 AS go-build-env
WORKDIR /app

# <- COPY go.mod and go.sum files to the workspace
COPY go.mod .
COPY go.sum .

RUN go mod download

# COPY the source code as the last step
COPY api ./api
COPY .git /.git

WORKDIR /app/api

# Build the Go app
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /helix

# Frontend build
FROM node:21-alpine AS ui-build-env

WORKDIR /app

RUN echo "installing apk packages" && \
  apk update && \
  apk upgrade && \
  apk add \
    bash \
    git \
    curl \
    openssh

# root config
COPY ./frontend/*.json /app/
COPY ./frontend/yarn.lock /app/yarn.lock

# Install modules
RUN yarn install

# Copy the rest of the code
COPY ./frontend /app

# Build the frontend
RUN yarn build

FROM alpine:3.17
RUN apk --update add ca-certificates

COPY --from=go-build-env /helix /helix
COPY --from=ui-build-env /app/dist /www

ENV FRONTEND_URL=/www

ENTRYPOINT ["/helix", "serve"]
