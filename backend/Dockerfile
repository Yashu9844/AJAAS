# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree
COPY . .

# Compile binary statically (no CGO runtime dependencies)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/server cmd/main.go

# Production Runner Stage
FROM alpine:3.19

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/server /app/server
COPY --from=builder /app/configs /app/configs

EXPOSE 8080

CMD ["/app/server"]
