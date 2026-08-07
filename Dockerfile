# Stage 1: Build binary
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install git and certificates
RUN apk add --no-cache git ca-certificates

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o main cmd/server/main.go

# Stage 2: Create minimal final image
FROM alpine:3.21

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

# Copy compiled binary
COPY --from=builder /app/main .

# Set permissions
RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE 8082

ENV TZ=Asia/Bangkok

CMD ["./main"]
