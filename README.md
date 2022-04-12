# goproxy-s3

[![Go](https://github.com/rakyll/goproxy-s3/actions/workflows/go.yml/badge.svg)](https://github.com/rakyll/goproxy-s3/actions/workflows/go.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/rakyll/goproxy-s3/proxy.svg)](https://pkg.go.dev/github.com/rakyll/goproxy-s3/proxy)

A Go module proxy that serves modules from S3.

Note: The project is not yet stable, there could be breaking changes.

```
$ goproxy-s3 -bucket=my-go-proxy-bucket -region=us-west-2
goproxy-s3: 2022/03/29 09:43:32 Proxy server is starting at ":8080"; set GOPROXY
goproxy-s3: 2022/03/29 09:43:32 Admin server is starting at ":9999"
```

To copy a package and its transient dependencies to S3, send a POST request to
the admin endpoint. An example:
```
$ curl -X POST http://localhost:9999/golang.org/x/text@v0.3.7
```

Set the GOPROXY to the goproxy-s3 endpoint and your modules will be served from S3.

```
$ GOPROXY=http://localhost:8080 go get golang.org/x/text@v0.3.7
```

Versions that are not available at S3 won't be served:

```
GOPROXY=http://localhost:8080 go get golang.org/x/text@v0.3.1
go get: golang.org/x/text@v0.3.1: reading http://localhost:8080/golang.org/x/text/@v/v0.3.1.info: 404 Not Found
```
