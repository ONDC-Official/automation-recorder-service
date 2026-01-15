# syntax=docker/dockerfile:1

FROM golang:alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Build args for metadata
ARG VERSION=dev
ARG COMMIT=unknown

# Copy and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT}" \
    -o automation-recorder .

# Minimal runtime
FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/automation-recorder /
COPY --from=builder /app/.env.example /.env.example

EXPOSE 8089 8090

USER nonroot:nonroot

ENTRYPOINT ["/automation-recorder"]