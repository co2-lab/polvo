package server

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/co2-lab/polvo/internal/doctor"
)

// fixRequest is the body for POST /api/doctor/fix.
type fixRequest struct {
	Label string `json:"label"`
}

// DiagnosisJSON is the JSON-serialisable form of a single diagnostic check.
type DiagnosisJSON struct {
	Category string `json:"category"`
	Label    string `json:"label"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail,omitempty"`
	Fix      string `json:"fix,omitempty"`
	Fixable  bool   `json:"fixable,omitempty"`
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	in := doctor.Input{}
	if s.deps != nil {
		in.Cfg = s.deps.Cfg
		in.Registry = s.deps.Registry
		in.Resolver = s.deps.Resolver
		if s.deps.IsWatching != nil {
			in.Watching = s.deps.IsWatching()
		}
	}

	stateFile := ".polvo/state.yaml"
	in.StateFile = stateFile
	_, err := os.Stat(stateFile)
	in.StateOK = err == nil

	diags := doctor.Run(in)

	out := make([]DiagnosisJSON, len(diags))
	for i, d := range diags {
		out[i] = DiagnosisJSON{
			Category: d.Category,
			Label:    d.Label,
			OK:       d.OK,
			Detail:   d.Detail,
			Fix:      d.Fix,
			Fixable:  d.Fixable,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handleDoctorFix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req fixRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Label == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	in := doctor.Input{}
	if s.deps != nil {
		in.Cfg = s.deps.Cfg
		in.Registry = s.deps.Registry
		in.Resolver = s.deps.Resolver
		if s.deps.IsWatching != nil {
			in.Watching = s.deps.IsWatching()
		}
	}
	stateFile := ".polvo/state.yaml"
	in.StateFile = stateFile
	_, err := os.Stat(stateFile)
	in.StateOK = err == nil

	diags := doctor.Run(in)
	for _, d := range diags {
		if d.Label == req.Label && d.Fixable && d.FixFn != nil {
			if err := d.FixFn(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	http.Error(w, "check not found or not fixable", http.StatusNotFound)
}

func (s *Server) handleExit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}()
}
