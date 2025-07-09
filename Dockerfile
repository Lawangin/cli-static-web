# Step 1: Build the Go binary
FROM golang:1.22-alpine AS builder

# Set destination for COPY commands
WORKDIR /app

# Copy and download module dependencies
COPY go.mod ./
#COPY go.sum ./
RUN go mod download

# Copy the rest of the app
COPY . ./

# Build the CLI binary
RUN GOOS=linux GOARCH=amd64 go build -o cli-static-web main.go

# Step 2: Create a minimal image
FROM alpine:latest

# Add CA certificates (often needed for network calls)
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/cli-static-web .

RUN chmod +x cli-static-web

# Make it executable
ENTRYPOINT ["./cli-static-web"]
