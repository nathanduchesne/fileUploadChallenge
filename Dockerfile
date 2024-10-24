# Use official Golang image to build the application
FROM golang:1.23.2-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application with optimizations to reduce binary size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /app/api api.go

# Now create a smaller image for running the app
FROM alpine:latest

# Set the working directory for the final image
WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/api .

# Expose the port your service is running on
EXPOSE 8080

# Command to run the application
CMD ["./api"]