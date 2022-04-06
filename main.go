// Copyright 2022 Jaana Dogan

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// 	http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main contains a program that serves Go modules from S3.
package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/rakyll/goproxy-s3/proxy"
)

var (
	listen string
	admin  string // Admin API...

	region   string
	endpoint string
	bucket   string
)

func main() {
	flag.StringVar(&listen, "listen", ":8080", "")
	flag.StringVar(&admin, "admin", ":9999", "")
	flag.StringVar(&region, "region", "", "")
	flag.StringVar(&endpoint, "endpoint", "", "")
	flag.StringVar(&bucket, "bucket", "", "")
	flag.Parse()

	log.SetPrefix("goproxy-s3: ")

	if bucket == "" {
		log.Fatalln("Please provide an S3 bucket name")
	}

	cfg := &aws.Config{}
	if region != "" {
		cfg.Region = aws.String(region)
	}
	if endpoint != "" {
		cfg.Endpoint = aws.String(endpoint)
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
