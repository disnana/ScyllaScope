package api

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

type asset struct {
	body, zipped []byte
	mime, tag    string
}

// Pre-compress the immutable embedded assets once. Never cache API or session data.
func newAssets(root fs.FS) http.Handler {
	assets := map[string]asset{}
	g := gzip.NewWriter(io.Discard)
	fs.WalkDir(root, "static", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, e := fs.ReadFile(root, name)
		if e != nil {
			return nil
		}
		mt := mime.TypeByExtension(path.Ext(name))
		if mt == "" {
			mt = "application/octet-stream"
		}
		var z bytes.Buffer
		g.Reset(&z)
		_, _ = g.Write(b)
		_ = g.Close()
		sum := sha256.Sum256(b)
		assets[strings.TrimPrefix(name, "static")] = asset{body: b, zipped: z.Bytes(), mime: mt, tag: fmt.Sprintf(`"%x"`, sum[:16])}
		return nil
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", 405)
			return
		}
		p := r.URL.Path
		if p == "/" {
			p = "/index.html"
		}
		a, ok := assets[p]
		if !ok || strings.HasSuffix(p, ".map") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", a.mime)
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		w.Header().Set("Vary", "Accept-Encoding")
		tag := a.tag
		b := a.body
		for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
			if strings.TrimSpace(enc) == "gzip" && len(a.zipped) < len(a.body) {
				b = a.zipped
				w.Header().Set("Content-Encoding", "gzip")
				tag = strings.TrimSuffix(tag, `"`) + `-gzip"`
				break
			}
		}
		w.Header().Set("ETag", tag)
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		// ServeContent handles HEAD and range requests without permitting directory listings.
		http.ServeContent(w, r, path.Base(p), time.Time{}, bytes.NewReader(b))
	})
}
