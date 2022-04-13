FROM golang:1.18

WORKDIR /build

COPY go.mod .
COPY go.sum .
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /goproxy-s3 .

ENTRYPOINT ["/goproxy-s3"]
