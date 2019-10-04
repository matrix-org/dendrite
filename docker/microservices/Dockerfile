FROM golang:1.12.9

WORKDIR /app

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Statically link binaries
ENV CGO_ENABLED=0
# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Build the Go app
RUN ./build.sh