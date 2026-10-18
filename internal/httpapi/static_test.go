package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedAssetsAndSPAFallbackDoNotRedirect(t *testing.T) {
	server := &Server{assets: fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<!doctype html><title>Wisp</title>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('wisp')")},
	}}
	for _, tc := range []struct{ url, contains string }{
		{"/", "<title>Wisp</title>"},
		{"/assets/app.js", "console.log"},
		{"/chat/somewhere", "<title>Wisp</title>"},
	} {
		t.Run(tc.url, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.url, nil)
			rec := httptest.NewRecorder()
			server.static(rec, req)
			if rec.Code != 200 || rec.Header().Get("Location") != "" || !strings.Contains(rec.Body.String(), tc.contains) {
				t.Fatal(rec.Code, rec.Header(), rec.Body.String())
			}
			if req.URL.Path != tc.url {
				t.Fatal("request URL was changed", req.URL.Path)
			}
		})
	}
}
