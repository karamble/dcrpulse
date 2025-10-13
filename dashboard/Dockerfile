# Stage 1: Build frontend with Node.js
FROM node:18-alpine AS frontend-builder

WORKDIR /app/web

# Copy frontend package files
COPY web/package*.json ./

# Install frontend dependencies
RUN npm install

# Copy frontend source code
COPY web/ ./

# Build frontend
RUN npm run build

# Stage 2: Build Go backend
FROM golang:1.24-alpine AS go-builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download Go dependencies
RUN go mod download

# Copy Go source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Copy built frontend files from previous stage
COPY --from=frontend-builder /app/web/dist ./cmd/dcrpulse/web/dist

# Build Go binary with embedded frontend
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo ./cmd/dcrpulse

# Stage 3: Runtime
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from go-builder
COPY --from=go-builder /app/dcrpulse .

# Expose port
EXPOSE 8080

# Run the application
CMD ["./dcrpulse"]

