# Containerfile to build all the packages
# which one is based on the source path variable below.

# Stage 1: Build the Go application
# We use a specific version of the Go image for stability.
FROM golang:1 AS builder


# Set the working directory inside the container
WORKDIR /go/src/github.com/papaburgs/fluffy-robot

# Copy the Go module files and download dependencies.
# This step is cached if go.mod and go.sum don't change, speeding up builds.
# We copy from the specified SOURCE_PATH.
COPY go.mod go.sum ./

# Copy the rest of the application source code from the specified path
COPY . .

RUN go mod tidy; go mod download


# Build the Go application.
# -o app: Specifies the output executable name as 'app'.
# -ldflags "-s -w": Reduces the size of the binary by stripping debug information and symbol tables.
# CGO_ENABLED=0: Disables CGO, making the binary statically linked and truly independent of the base image.
RUN CGO_ENABLED=0 GOOS=linux go build -o app -ldflags "-s -w" *.go

# Create the final minimal image
# We use 'scratch' which is an empty base image, smaller footprint
FROM scratch

# Set the working directory in the final image
WORKDIR /root/

# Copy the compiled executable from the 'builder' stage
COPY --from=builder /go/src/github.com/papaburgs/fluffy-robot/app .
COPY --from=builder /go/src/github.com/papaburgs/fluffy-robot/templates ./templates/
COPY --from=builder /go/src/github.com/papaburgs/fluffy-robot/static ./static/
# Copy the CA certificates from the builder stage
# Standard location for many applications to find them
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Command to run the executable when the container starts
CMD ["./app"]
