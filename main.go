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

	provider string // s3, gcs, etc
	region   string
	bucket   string
	endpoint string // cloud API endpoint
)

func main() {
	flag.StringVar(&listen, "listen", ":8080", "")
	flag.StringVar(&admin, "admin", ":9999", "")

	flag.StringVar(&provider, "provider", "s3", "")
	flag.StringVar(&region, "region", "", "")
	flag.StringVar(&bucket, "bucket", "", "")
	flag.StringVar(&endpoint, "endpoint", "", "")

	flag.Parse()

	log.SetPrefix("goproxy-s3: ")

	if bucket == "" {
		log.Fatalln("Please provide a bucket name")
	}

	var downloader proxy.Downloader
	var copier proxy.Copier
	switch provider {
	case "s3":
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
		copier = proxy.NewS3Copier(sess, bucket)
		downloader = proxy.NewS3Downloader(sess, bucket)
	default:
		log.Fatalf("Unknown provider: %q", provider)
	}

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
			Downloader: downloader,
		},
	}
	log.Printf("Proxy server is starting at %q; set GOPROXY", listen)
	log.Fatalln(server.ListenAndServe())
}
