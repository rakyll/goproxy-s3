package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"golang.org/x/mod/module"
)

const (
	contentTypeJSON   = "application/json"
	contentTypeText   = "text/plain; charset=UTF-8"
	contentTypeBinary = "application/octet-stream"
)

const errCodeNotFound = "NotFound" // See https://github.com/aws/aws-sdk-go/issues/1208.

// Downloader reads a Go module from an S3 bucket.
// Use NewDownloader to initialize one.
type Downloader struct {
	bucket string
	client *s3.S3
}

func NewDownloader(s *session.Session, bucket string) *Downloader {
	return &Downloader{
		bucket: bucket,
		client: s3.New(s),
	}
}

// Download downloads a module from an S3 bucket. modulePath is the import
// path of the module, e.g. golang.org/x/text. name is the asset's name such as
// v0.3.0.info, v0.3.0.mod, v0.3.0.ziphash, or v0.3.0.zip.
func (d *Downloader) Download(modulePath string, name string) (io.ReadCloser, error) {
	o, err := d.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(fmt.Sprintf("modules/%s/@v/%s", modulePath, name)),
	})
	if err != nil {
		return nil, err
	}
	return o.Body, nil
}

// Copier copies a module to S3. Use NewCopier to initiate one.
type Copier struct {
	// TODO(jbd): Allow Copier to be an abstract type to support
	// vendors other than S3.
	bucket   string
	uploader *s3manager.Uploader
}

func NewCopier(s *session.Session, bucket string) *Copier {
	uploader := s3manager.NewUploader(s)
	return &Copier{
		bucket:   bucket,
		uploader: uploader,
	}
}

// Copy will run go mod download locally for the given
// module and upload artifacts to S3. Copy will
// ensure all transient dependencies are copied.
func (c *Copier) Copy(m module.Version) error {
	log.Printf("Resolving module: %s", m)
	info, err := goModDownload(m)
	if err != nil {
		return err
	}
	defer os.RemoveAll(info.Cache)

	// Downloads all transient dependencies.
	if err := goModDownloadAll(info.Cache, info.GoMod); err != nil {
		return err
	}

	assetsDir := filepath.Join(info.Cache, "cache", "download")
	if err := filepath.Walk(assetsDir, func(path string, info fs.FileInfo, err error) error {
		if !shouldUpload(info) {
			return nil
		}
		o := strings.Replace(path, assetsDir, "", 1)
		return c.upload(path, o)
	}); err != nil {
		return err
	}
	return nil
}

func (c *Copier) upload(src string, dest string) error {
	f, err := os.OpenFile(src, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	key := "modules" + dest

	log.Printf("Checking if %q exists", key)
	_, err = c.uploader.S3.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == errCodeNotFound {
			log.Printf("Uploading %q", key)
			_, err = c.uploader.Upload(&s3manager.UploadInput{
				Bucket: aws.String(c.bucket),
				Key:    aws.String(key),
				Body:   f,
			})
		}
	}
	return err
}

func shouldUpload(fi os.FileInfo) bool {
	if fi.IsDir() {
		return false
	}
	name := fi.Name()
	if name == "list" {
		return true
	}
	ext := filepath.Ext(name)
	if ext == ".mod" || ext == ".zip" || ext == ".ziphash" || ext == ".info" {
		return true
	}
	if strings.Contains(name, "sumdb") {
		return true
	}
	return false
}

func (c *Copier) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// POST http://localhost:9999/golang.org/x/text@v3.0.1
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	path, version, ok := parseURLPathForModule(r.URL.Path)
	if !ok {
		http.Error(w, "malformed module path or version", http.StatusBadRequest)
		return
	}
	if err := c.Copy(module.Version{Path: path, Version: version}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

type moduleInfo struct {
	Path    string
	Version string
	GoMod   string
	Cache   string
}

func goModDownload(m module.Version) (*moduleInfo, error) {
	cache, err := ioutil.TempDir("", "go-mod-download")
	if err != nil {
		return nil, err
	}

	var info moduleInfo
	info.Cache = cache

	cmd := exec.Command("go", "mod", "download", "-json", m.String())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"GOMODCACHE="+cache,
	)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s%s", stderr.String(), stdout.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return nil, fmt.Errorf("error reading json: %v", err)
	}
	return &info, nil
}

// goModDownloadAll downloads all of the transient
// dependencies of a module to the given cache, it requires module zip.
func goModDownloadAll(cache, gomod string) error {
	// TODO(jbd): Investigate whether there is a better way to
	// download all dependencies.
	moduleSource, err := ioutil.TempDir("", "go-mod-source")
	if err != nil {
		return err
	}
	defer os.RemoveAll(moduleSource)

	gomodBytes, err := ioutil.ReadFile(gomod)
	if err != nil {
		return err
	}
	dst := filepath.Join(moduleSource, "go.mod")
	if err := os.WriteFile(dst, gomodBytes, 0644); err != nil {
		return err
	}

	cmd := exec.Command("go", "mod", "download", "-json", "all")
	cmd.Dir = moduleSource
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"GOMODCACHE="+cache,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s%s", stderr.String(), stdout.String())
	}
	return nil
}

func parseURLPathForModule(urlPath string) (path, version string, ok bool) {
	urlPath = strings.TrimPrefix(urlPath, "/")
	i := strings.Index(urlPath, "@")
	if i < 0 {
		return "", "", false
	}
	return urlPath[:i], urlPath[i+1:], true
}
