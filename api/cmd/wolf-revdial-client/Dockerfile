FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy go.mod and go.sum from root
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY api/ api/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o wolf-revdial-client ./api/cmd/wolf-revdial-client/

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

COPY --from=builder /build/wolf-revdial-client /usr/local/bin/wolf-revdial-client

ENTRYPOINT ["/usr/local/bin/wolf-revdial-client"]
