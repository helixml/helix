### API Base ###
#---------------
FROM golang:1.24-alpine AS api-base
WORKDIR /app
# Install git for development and build environments
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download

### API Development ###
#----------------------
FROM api-base AS api-dev-env
# - Air provides hot reload for Go
RUN go install github.com/air-verse/air@v1.52.3
# - Install curl for Wolf API debugging, bash for git operations, and git-daemon for git-http-backend
RUN apk add --no-cache curl bash git-daemon
# - Copy the files and run a build to make startup faster
COPY api /app/api
WORKDIR /app/api
# - Run a build to make the intial air build faster
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /helix
# - Entrypoint is the air command
ENTRYPOINT ["air", "--build.bin", "/helix", "--build.cmd", "CGO_ENABLED=0 go build -ldflags \"-s -w\" -o /helix", "--build.stop_on_error", "true", "--"]
CMD ["serve"]


#### API Build ###
#-----------------------
FROM api-base AS api-build-env
# Following git lines required for buildvcs to work
RUN apk add --no-cache git
COPY .git /app/.git
COPY api /app/api
WORKDIR /app/api
# - main.version is a variable required by Sentry and is set in .drone.yaml
ARG APP_VERSION="v0.0.0+unknown"
RUN CGO_ENABLED=0 go build -buildvcs=true -ldflags "-s -w -X main.version=$APP_VERSION -X github.com/helixml/helix/api/pkg/data.Version=$APP_VERSION" -o /helix

### Frontend Base ###
#--------------------
FROM node:25-alpine AS ui-base
WORKDIR /app
# - Install dependencies
COPY ./frontend/*.json /app/
COPY ./frontend/yarn.lock /app/yarn.lock
RUN yarn install


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
FROM alpine:3.21
RUN apk --update add --no-cache ca-certificates git

COPY --from=api-build-env /helix /helix
COPY --from=ui-build-env /app/dist /www

ENV FRONTEND_URL=/www

EXPOSE 80

ENTRYPOINT ["/helix", "serve"]
