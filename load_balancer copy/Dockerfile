FROM --platform=linux/amd64 golang:1.19.5

# RUN apk add --no-cache git

# Set the Current Working Directory inside the container
WORKDIR /app/go_server

# We want to populate the module cache based on the go.{mod,sum} files.
COPY go.mod .

RUN go mod download

COPY . .

# Build the Go app
RUN go build -o ./out/go_server .

# This container exposes port 3000 to the outside world
EXPOSE 3000

# Run the binary program produced by `go install`
CMD ["./out/go_server"]
