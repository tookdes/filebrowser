package web

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gtsteffaniak/filebrowser/backend/internal/database/users"
	"github.com/gtsteffaniak/filebrowser/backend/internal/state"
	"github.com/gtsteffaniak/filebrowser/backend/internal/utils"
)

// triageItemPatchRequest is the JSON body for PATCH /api/users/triage-items.
type triageItemPatchRequest struct {
	Source string `json:"source" validate:"required"`
	Path   string `json:"path" validate:"required"` // scope-relative parent directory
	Name   string `json:"name" validate:"required"` // item basename within path
	Status string `json:"status"`                    // keep, reject, or empty to clear
}

// userPatchTriageItemsHandler sets or clears the per-user review state of one file or folder.
func userPatchTriageItemsHandler(w http.ResponseWriter, r *http.Request, d *Context) (int, error) {
	var body triageItemPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to decode body: %w", err)
	}
	defer r.Body.Close()

	if body.Source == "" || body.Path == "" || body.Name == "" {
		return http.StatusBadRequest, fmt.Errorf("source, path, and name are required")
	}

	status, ok := users.NormalizeTriageStatus(body.Status)
	if !ok {
		return http.StatusBadRequest, fmt.Errorf("status must be keep, reject, or empty")
	}

	cleanPath, err := utils.SanitizePath(body.Path)
	if err != nil {
		return http.StatusBadRequest, err
	}
	body.Path = cleanPath

	cleanName, err := utils.SanitizePath(body.Name)
	if err != nil {
		return http.StatusBadRequest, err
	}
	body.Name = cleanName

	source, ok := users.ResolveSourceKey(body.Source)
	if !ok {
		return http.StatusBadRequest, fmt.Errorf("source not found: %s", body.Source)
	}
	if !d.User.HasSourceByPath(source.Path) {
		return http.StatusForbidden, fmt.Errorf("access denied to source %s", body.Source)
	}

	userScope, err := d.User.GetScopeForSourcePath(source.Path)
	if err != nil {
		return http.StatusForbidden, err
	}

	sourcePath, indexDirPath, err := scopeRelativeDirToIndexPath(body.Source, userScope, body.Path)
	if err != nil {
		return http.StatusBadRequest, err
	}

	u, err := state.GetUserByID(d.User.ID)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	u.EnsureTriageItems().Set(sourcePath, indexDirPath, body.Name, status)
	if err := state.UpdateUser(&u, "", "TriageItems"); err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusNoContent, nil
}
