FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install docker CLI
RUN apk add --no-cache docker-cli

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates docker-cli

WORKDIR /root/

# Copy binary and static files
COPY --from=builder /app/server .
COPY index.html .
COPY app.js .

EXPOSE 8080

CMD ["./server"]
