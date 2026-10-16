package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"wisp/internal/events"
	"wisp/internal/store"
	"wisp/internal/whatsapp"
)

func multipartImage(t *testing.T, count, size int) *http.Request {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for i := 0; i < count; i++ {
		file, _ := w.CreateFormFile("image", "photo.png")
		_, _ = file.Write(bytes.Repeat([]byte{'x'}, size))
	}
	_ = w.WriteField("caption", "caption")
	_ = w.WriteField("reply_to", "original")
	_ = w.Close()
	req := httptest.NewRequest("POST", "/api/chats/g@g.us/send-image", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}
func TestReadImageUploadLimits(t *testing.T) {
	req := multipartImage(t, 1, 4)
	data, caption, reply, err := readImageUpload(httptest.NewRecorder(), req)
	if err != nil || string(data) != "xxxx" || caption != "caption" || reply != "original" {
		t.Fatal(caption, reply, err)
	}
	for _, req := range []*http.Request{multipartImage(t, 2, 4), multipartImage(t, 0, 4), multipartImage(t, 1, whatsapp.MaxImageBytes+1), httptest.NewRequest("POST", "/", bytes.NewBufferString("broken"))} {
		if _, _, _, err := readImageUpload(httptest.NewRecorder(), req); err == nil {
			t.Fatal("invalid multipart accepted")
		}
	}
}
func TestImageEndpointRejectsReadOnlyAndInvalidImages(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "wisp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	bus := events.New()
	wa := whatsapp.New(st, bus, whatsapp.Options{MediaDir: t.TempDir()})
	handler := New(st, wa, bus, nil, nil, nil, "").Handler()
	for _, tt := range []struct {
		mirror     bool
		size, code int
	}{{false, 4, 400}, {false, whatsapp.MaxImageBytes + 1, 413}, {true, 4, 403}} {
		if tt.mirror {
			st.SetConfigOverride("mirrorOnly", "true")
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, multipartImage(t, 1, tt.size))
		if rec.Code != tt.code {
			t.Fatalf("status %d want %d: %s", rec.Code, tt.code, rec.Body.String())
		}
	}
}
