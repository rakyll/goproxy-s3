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

package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
)

const (
	contentTypeJSON   = "application/json"
	contentTypeText   = "text/plain; charset=UTF-8"
	contentTypeBinary = "application/octet-stream"
)

const errCodeNotFound = "NotFound" // See https://github.com/aws/aws-sdk-go/issues/1208.

type Downloader interface {
	Download(modulePath string, name string) (io.ReadCloser, error)
}

type Copier interface {
	Copy(force bool, m module.Version) error
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	// TODO(jbd): Remove ServeHTTP from Copier.
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
