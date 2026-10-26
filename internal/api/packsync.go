package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/tung1998/agent-office/internal/perm"
	"github.com/tung1998/agent-office/internal/prompts"
	"github.com/tung1998/agent-office/internal/storage"
	"github.com/tung1998/agent-office/internal/transfer"
	"github.com/tung1998/agent-office/templates/packs"
)

// Bringing a project up to a newer version of the library template it was
// made from (ADR-206): a preview of what would change, then either office
// applies the template (settings by key and name, its skill files), or the
// project's default agent gets the preview and the template and brings the
// project up to it with judgement, through cards a person approves.

// packKey is where a project keeps the library template it was made from.
func packKey(projectID string) string { return "pack:" + projectID }

// projectPack is the template a project was made from ("" = not known).
func (s *server) projectPack(ctx context.Context, projectID string) string {
	var ref string
	if ok, err := s.cfg.Store.Settings().Get(ctx, packKey(projectID), &ref); err != nil || !ok {
		return ""
	}
	return ref
}

// packSyncPreview: GET ?ref= (default: the project's template).
func (s *server) packSyncPreview(w http.ResponseWriter, r *http.Request) {
	x, err := s.cfg.Store.Repos().Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ref == "" {
		ref = s.projectPack(r.Context(), x.ID)
	}
	out := map[string]any{"ref": ref, "known": s.projectPack(r.Context(), x.ID), "changes": []transfer.Change{}, "files": []string{}}
	if ref == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	res, files, err := s.packSync(r.Context(), x, ref, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out["changes"], out["files"] = res.Changes, nonNilStrings(files)
	writeJSON(w, http.StatusOK, out)
}

// packSyncApply: POST {ref, mode: apply | ai}.
func (s *server) packSyncApply(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ref  string `json:"ref"`
		Mode string `json:"mode"`
	}
	if !decode(w, r, &in) {
		return
	}
	x, err := s.cfg.Store.Repos().Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeDomainError(w, r, err)
		return
	}
	ref := strings.TrimSpace(in.Ref)
	if ref == "" {
		ref = s.projectPack(r.Context(), x.ID)
	}
	if ref == "" {
		writeError(w, http.StatusBadRequest, "Chọn mẫu để đồng bộ")
		return
	}
	switch in.Mode {
	case "apply":
		res, files, err := s.packSync(r.Context(), x, ref, false)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		_ = s.cfg.Store.Settings().Set(r.Context(), packKey(x.ID), ref)
		s.auditAction(r, "project.pack_sync", x.ID, map[string]any{"ref": ref, "mode": "apply", "files": files})
		writeJSON(w, http.StatusOK, map[string]any{"ref": ref, "changes": res.Changes, "files": nonNilStrings(files)})
	case "ai":
		// the template's files first (the agent reads the skills it brings),
		// then the agent weighs the settings against what the project became
		res, err := s.packSyncSettings(r.Context(), x, ref, true)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		files, err := packs.SyncFiles(r.Context(), ref, x.Path, x.Path == "")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		c, err := s.cfg.Chat.NewConversation(r.Context(), x.ID, "")
		if err != nil {
			s.chatError(w, r, err)
			return
		}
		var lines []string
		for _, ch := range res.Changes {
			if ch.Op == "unchanged" && ch.Detail == "" {
				continue
			}
			line := "- " + ch.Kind + " " + ch.Name + ": " + ch.Op
			if ch.Detail != "" {
				line += " (" + ch.Detail + ")"
			}
			lines = append(lines, line)
		}
		_ = s.cfg.Chat.SetMode(r.Context(), c.ID, perm.Operate) // it changes settings on its own as its rights allow
		text := prompts.Render("chat/pack-sync", map[string]any{"Ref": ref, "Changes": lines, "Files": files})
		if _, _, err := s.cfg.Chat.Send(r.Context(), c.ID, text, nil); err != nil {
			s.chatError(w, r, err)
			return
		}
		_ = s.cfg.Store.Settings().Set(r.Context(), packKey(x.ID), ref)
		s.auditAction(r, "project.pack_sync", x.ID, map[string]any{"ref": ref, "mode": "ai", "files": files, "conversation": c.ID})
		writeJSON(w, http.StatusOK, map[string]any{"ref": ref, "conversation_id": c.ID, "files": nonNilStrings(files)})
	default:
		writeError(w, http.StatusBadRequest, "mode là apply hoặc ai")
	}
}

// packSync previews (dryRun) or applies template ref to x: its settings and
// its files.
func (s *server) packSync(ctx context.Context, x storage.Repo, ref string, dryRun bool) (transfer.Result, []string, error) {
	res, err := s.packSyncSettings(ctx, x, ref, dryRun)
	if err != nil {
		return res, nil, err
	}
	if x.Path == "" {
		return res, nil, nil
	}
	files, err := packs.SyncFiles(ctx, ref, x.Path, dryRun)
	return res, files, err
}

func (s *server) packSyncSettings(ctx context.Context, x storage.Repo, ref string, dryRun bool) (transfer.Result, error) {
	if s.cfg.Transfer == nil {
		return transfer.Result{}, errors.New("office này không đồng bộ mẫu")
	}
	raw, err := packs.Read(ref)
	if err != nil {
		return transfer.Result{}, err
	}
	p, err := transfer.ReadPack(raw)
	if err != nil {
		return transfer.Result{}, err
	}
	return s.cfg.Transfer.SyncPack(ctx, p, x, dryRun)
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
