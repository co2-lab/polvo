package server

import (
	"encoding/json"
	"net/http"

	"github.com/co2-lab/polvo/internal/clidetect"
)

func (s *Server) handleCLIs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clis := clidetect.Detect()
	if clis == nil {
		clis = []clidetect.CLI{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clis) //nolint:errcheck
}
