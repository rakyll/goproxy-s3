// Package main contains a program that serves Go modules from S3.
package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/rakyll/goproxyproxy/proxy"
)

var (
	listen string
	admin  string // Admin API...

	region string
	bucket string
)

func main() {
	flag.StringVar(&listen, "listen", ":8080", "")
	flag.StringVar(&admin, "admin", ":9999", "")
	flag.StringVar(&region, "region", "", "")
	flag.StringVar(&bucket, "bucket", "", "")
	flag.Parse()

	log.SetPrefix("goproxyproxy: ")

	if bucket == "" {
		log.Fatalln("Please provide an S3 bucket name")
	}

	cfg := &aws.Config{}
	if region != "" {
		cfg.Region = aws.String(region)
	}
	sess, err := session.NewSession(cfg)
	if err != nil {
		log.Fatalf("Cannot create AWS session: %v", err)
	}

	copier := proxy.NewCopier(sess, bucket)
	adminServer := http.Server{
		Addr:    admin,
		Handler: copier,
	}
	go func() {
		log.Printf("Admin server is starting at %q", admin)
		log.Fatalln(adminServer.ListenAndServe())
	}()

	server := http.Server{
		Addr: listen,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true, // TODO(jbd): Support TLS options.
		},
		Handler: &proxy.ProxyHandler{
			Downloader: proxy.NewDownloader(sess, bucket),
		},
	}
	log.Printf("Proxy server is starting at %q; set GOPROXY", listen)
	log.Fatalln(server.ListenAndServe())
}
