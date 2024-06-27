### API Base ###
#---------------
FROM golang:1.22-alpine AS api-base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

### API Development ###
#----------------------
FROM api-base as api-dev-env
# - Air provides hot reload for Go
RUN go install github.com/air-verse/air@v1.52.3
# - Copy the files and run a build to make startup faster
COPY api /app/api
WORKDIR /app/api
# - Run a build to make the intial air build faster
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /helix
# - Entrypoint is the air command
ENTRYPOINT ["air", "--build.bin", "/helix", "--build.cmd", "CGO_ENABLED=0 go build -ldflags \"-s -w\" -o /helix", "--"]
CMD ["serve"]


#### API Build ###
#-----------------------
FROM api-base AS api-build-env
COPY api /app/api
WORKDIR /app/api
# - main.version is a variable required by Sentry and is set in .drone.yaml
ARG APP_VERSION="v0.0.0+unknown" 
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$APP_VERSION" -o /helix

### Frontend Base ###
#--------------------
FROM node:21-alpine AS ui-base
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
FROM alpine:3.17
RUN apk --update add --no-cache ca-certificates

COPY --from=api-build-env /helix /helix
COPY --from=ui-build-env /app/dist /www

ENV FRONTEND_URL=/www

EXPOSE 80

ENTRYPOINT ["/helix", "serve"]
