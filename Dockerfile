FROM golang:alpine AS builder

# Set necessary environmet variables needed for our image
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Move to working directory /build
WORKDIR /build

# set go proxy
RUN go env -w GOPROXY=https://mirrors.aliyun.com/goproxy/,direct

# add gcc
RUN apk add build-base

# Copy and download dependency using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
RUN go build -o main cmd/byrbt-api/api.go

# Move to /dist directory as the place for resulting binary folder
WORKDIR /dist

# Copy binary from build to main folder
RUN cp /build/main .

RUN mkdir -p download
RUN mkdir -p mount

# Build a small image
FROM scratch

COPY --from=builder /dist/main /
COPY --from=builder /dist/download /
COPY --from=builder /dist/mount /

# Command to run
ENTRYPOINT ["/main"]
CMD ["-downloadDir='/download'", "-mountDir='/mount'"]
