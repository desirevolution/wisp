package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
	"wisp/internal/whatsapp"
)

// Bounded multipart parsing accepts one file and two short text fields.
func readImageUpload(w http.ResponseWriter, r *http.Request) ([]byte, string, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, whatsapp.MaxImageBytes+(64<<10))
	if err := r.ParseMultipartForm(whatsapp.MaxImageBytes + (64 << 10)); err != nil {
		return nil, "", "", err
	}
	defer r.MultipartForm.RemoveAll()
	form := r.MultipartForm
	if len(form.File) != 1 || len(form.File["image"]) != 1 {
		return nil, "", "", whatsapp.ErrInvalidAction
	}
	for key, values := range form.Value {
		if (key != "caption" && key != "reply_to") || len(values) != 1 || len(values[0]) > 16<<10 {
			return nil, "", "", whatsapp.ErrInvalidAction
		}
	}
	caption, reply := "", ""
	if values := form.Value["caption"]; len(values) == 1 {
		caption = values[0]
	}
	if values := form.Value["reply_to"]; len(values) == 1 {
		reply = values[0]
	}
	if len(reply) > 512 {
		return nil, "", "", whatsapp.ErrInvalidAction
	}
	header := form.File["image"][0]
	if header.Size <= 0 || header.Size > whatsapp.MaxImageBytes {
		return nil, "", "", &http.MaxBytesError{Limit: whatsapp.MaxImageBytes}
	}
	file, err := header.Open()
	if err != nil {
		return nil, "", "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, whatsapp.MaxImageBytes+1))
	return data, caption, reply, err
}

func (s *Server) sendImage(w http.ResponseWriter, r *http.Request, chat string) {
	if s.st.ConfigBool("mirrorOnly", false) {
		jsonOut(w, 403, map[string]string{"error": "Wisp is in mirror-only mode"})
		return
	}
	data, caption, reply, err := readImageUpload(w, r)
	if err != nil {
		code := 400
		msg := "Choose one PNG or JPEG image (maximum 10 MiB)"
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			code = 413
			msg = "Image upload exceeds 10 MiB"
		}
		jsonOut(w, code, map[string]string{"error": msg})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	id, err := s.wa.SendImage(ctx, chat, caption, reply, data, r.Header.Get("X-Wisp-Message-ID"))
	if err != nil {
		code := 502
		if errors.Is(err, whatsapp.ErrInvalidAction) {
			code = 400
		}
		jsonOut(w, code, map[string]string{"error": err.Error()})
		return
	}
	jsonOut(w, 200, map[string]any{"ok": true, "id": id})
}
