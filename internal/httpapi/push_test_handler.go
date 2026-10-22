package httpapi

import (
	"errors"
	"net/http"
	"wisp/internal/pushsvc"
)

func (s *Server) pushTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		jsonOut(w, 405, map[string]string{"error": "Use POST"})
		return
	}
	report, err := s.push.TestAll(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, pushsvc.ErrTestDisabled):
			jsonOut(w, 403, map[string]string{"error": "Push testing is disabled by deployment policy"})
		case errors.Is(err, pushsvc.ErrTestBusy):
			jsonOut(w, 409, map[string]string{"error": "A push test is already running"})
		default:
			jsonOut(w, 500, map[string]string{"error": "Could not run the push test"})
		}
		return
	}
	jsonOut(w, 200, report)
}
