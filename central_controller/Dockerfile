FROM --platform=linux/amd64 golang:1.19.5

# Set the Current Working Directory inside the container
WORKDIR /app/central_controller

# We want to populate the module cache based on the go.{mod,sum} files.
COPY go.mod .

RUN go mod download

COPY . .

# Build the Go app
RUN go build -o ./out/central_controller .

# This container exposes port 3000 to the outside world
EXPOSE 3000

# Run the binary program produced by `go install`
CMD ["./out/central_controller"]
