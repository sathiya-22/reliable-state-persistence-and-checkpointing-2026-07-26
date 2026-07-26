# Use the official Go image to build the application
FROM golang:1.21-alpine AS builder

# Set the current working directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are unchanged
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go application
RUN go build -o /agent-state-store .

# Start a new stage from scratch
FROM alpine:latest

# Install ca-certificates to allow HTTPS connections
RUN apk --no-cache add ca-certificates

# Set the current working directory inside the container
WORKDIR /root/

# Copy the pre-built binary from the builder stage
COPY --from=builder /agent-state-store .

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["./agent-state-store"]
