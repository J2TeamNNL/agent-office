package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/tung1998/agent-office/internal/audit"
	"github.com/tung1998/agent-office/internal/storage"
)

// The work board's Burn as a setting (ADR-196): an agent that runs the
// project (a company's CEO) steers it with propose_change resource burn:
// what it works on, who reviews, its schedule, and run on / off / finish.
// A person approves it, or it is done at once for an administrator's run.

// burnConfigInput is what propose_change may set on the Burn.
type burnConfigInput struct {
	AgentID         *string           `json:"agent_id"`
	ModelTier       *string           `json:"model_tier"`
	MaxParallel     *int              `json:"max_parallel"`
	Focus           *string           `json:"focus"`
	Order           *string           `json:"order"`
	Template        *string           `json:"template"`
	HuntPrompt      *string           `json:"hunt_prompt"`
	Verify          *string           `json:"verify"`
	ReviewCap       *int              `json:"review_cap"`
	StopAfter       *int              `json:"stop_after"`
	ReviewProfileID *string           `json:"review_profile_id"`
	Plan            *storage.BurnPlan `json:"plan"`
	// Run: on (turned on, it stops by its plan) | off | finish (finishes
	// what it has in progress, then stops)
	Run *string `json:"run"`
}

// burnConfigKind is the Burn in the config registry.
func burnConfigKind() cfgKind {
	return cfgKind{name: "burn", title: "Burn (bảng việc)", input: burnConfigInput{}, merge: false,
		get: func(s *server, ctx context.Context, _, projectID string) (any, string, error) {
			if s.cfg.Burn == nil {
				return nil, "", errNoBurn
			}
			b, err := s.cfg.Burn.Session(ctx, projectID)
			if err != nil {
				return nil, "", err
			}
			raw, _ := json.Marshal(toBurnDTO(b))
			var out map[string]any
			_ = json.Unmarshal(raw, &out)
			out["run"] = "off"
			if b.Active() {
				out["run"] = "on"
			}
			return out, projectID, nil
		},
		update: &cfgRoute{"PUT", func(s *server) http.HandlerFunc { return s.configBurn }, byProject}}
}

var errNoBurn = errors.New("Burn chưa bật trên office này")

// configBurn saves the Burn's settings, then turns it on or off as Run says.
func (s *server) configBurn(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Burn == nil {
		writeError(w, http.StatusNotImplemented, errNoBurn.Error())
		return
	}
	var in burnConfigInput
	if !decode(w, r, &in) {
		return
	}
	pid := r.PathValue("id")
	b, err := s.burnSession(r, pid)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	before := toBurnDTO(b)
	set := burnInput{AgentID: in.AgentID, ModelTier: in.ModelTier, MaxParallel: in.MaxParallel, Focus: in.Focus, Order: in.Order, Template: in.Template,
		HuntPrompt: in.HuntPrompt, Verify: in.Verify, ReviewCap: in.ReviewCap, StopAfter: in.StopAfter, ReviewProfileID: in.ReviewProfileID, Plan: in.Plan}
	if err := s.applyBurn(r, set, &b); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	run := ""
	if in.Run != nil {
		run = *in.Run
	}
	switch run {
	case "", "on", "off", "finish":
	default:
		writeError(w, http.StatusBadRequest, "run là on, off hoặc finish")
		return
	}
	if b, err = s.cfg.Store.Burn().SaveSession(r.Context(), b); err != nil {
		s.internal(w, r, err)
		return
	}
	s.audit(r, audit.Change{Action: "burn.update", Resource: "burn", ResourceID: b.ID, ProjectID: pid, Before: before, After: toBurnDTO(b)})
	switch {
	case run == "on" && !b.Active():
		b, err = s.cfg.Burn.TurnOn(r.Context(), pid, userFrom(r).Email, time.Now())
		s.audit(r, audit.Change{Action: "burn.start", Resource: "burn", ResourceID: b.ID, ProjectID: pid, After: toBurnDTO(b)})
	case run == "off" && b.Active():
		err = s.cfg.Burn.Stop(r.Context(), pid)
		s.audit(r, audit.Change{Action: "burn.stop", Resource: "burn", ProjectID: pid})
	case run == "finish" && b.Active():
		err = s.cfg.Burn.Drain(r.Context(), pid)
		s.audit(r, audit.Change{Action: "burn.drain", Resource: "burn", ProjectID: pid})
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"burn": toBurnDTO(b)})
}
