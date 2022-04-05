// Package proxy contains a Go proxy that serves modules from S3.
package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"golang.org/x/mod/module"
)

type ProxyHandler struct {
	Downloader *Downloader
	// TODO(jbd): Allow downloader to be an interface and multiple
	// vendor implementations are available.
}

// ServeHTTP implement a Go proxy server handler.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Disable sumdb.
	if idx := strings.Index(r.URL.Path, "sumdb/"); idx > 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	i := strings.Index(r.URL.Path, "/@v/")
	if i < 0 {
		http.Error(w, "no path", http.StatusBadRequest)
		return
	}

	modPath, err := module.UnescapePath(strings.TrimPrefix(r.URL.Path[:i], "/"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	what := r.URL.Path[i+len("/@v/"):]

	var ctype string
	var f io.ReadCloser
	switch what {
	case "latest":
		err = errors.New("latest is not supported")

	case "list":
		ctype = contentTypeText
		f, err = h.List(ctx, modPath)

	default:
		ext := path.Ext(what)

		var version string
		version, err = module.UnescapeVersion(strings.TrimSuffix(what, ext))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m := module.Version{Path: modPath, Version: version}
		if version == "latest" {
			http.Error(w, "version latest is disallowed", http.StatusBadRequest)
			return
		}

		if ext != ".info" && version != module.CanonicalVersion(version) {
			http.Error(w, "version "+version+" is not in canonical form", http.StatusBadRequest)
			return
		}

		switch ext {
		case ".info":
			ctype = contentTypeJSON
			f, err = h.Info(ctx, m)

		case ".mod":
			ctype = contentTypeText
			f, err = h.GoMod(ctx, m)

		case ".zip":
			ctype = contentTypeBinary
			f, err = h.Zip(ctx, m)

		default:
			http.Error(w, "request not recognized", http.StatusBadRequest)
			return
		}
	}

	if err != nil {
		h.handleError(w, r, err)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", ctype)
	if _, err := io.Copy(w, f); err != nil {
		h.handleError(w, r, err)
	}
}

func (h *ProxyHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	code := http.StatusInternalServerError
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == errCodeNotFound {
			code = http.StatusNotFound
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		code = http.StatusNotFound
	}
	http.Error(w, err.Error(), code)
}

// List returns the module listing. Module path is in the
// format of golang.org/x/text.
func (h *ProxyHandler) List(ctx context.Context, modulePath string) (io.ReadCloser, error) {
	path, err := module.EscapePath(modulePath)
	if err != nil {
		return nil, err
	}
	return h.Downloader.Download(path, "listproxy")
}

// Info returns the module .info for the specified version.
func (h *ProxyHandler) Info(ctx context.Context, m module.Version) (io.ReadCloser, error) {
	return h.Downloader.Download(m.Path, m.Version+".info")
}

// GoMod returns the module .mod for the specified version.
func (h *ProxyHandler) GoMod(ctx context.Context, m module.Version) (io.ReadCloser, error) {
	return h.Downloader.Download(m.Path, m.Version+".mod")
}

// Zip returns the module .zip for the specified version.
func (h *ProxyHandler) Zip(ctx context.Context, m module.Version) (io.ReadCloser, error) {
	return h.Downloader.Download(m.Path, m.Version+".zip")
}
