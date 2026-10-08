package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gtsteffaniak/filebrowser/backend/internal/database/users"
	"github.com/gtsteffaniak/filebrowser/backend/internal/state"
)

type displayPreferencesPatchRequest struct {
	Source  string         `json:"source"`
	Path    string         `json:"path"`
	Sorting *users.Sorting `json:"sorting,omitempty"`
}

func userPatchDisplayPreferencesHandler(w http.ResponseWriter, r *http.Request, d *Context) (int, error) {
	var body displayPreferencesPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return http.StatusBadRequest, fmt.Errorf("failed to decode body: %w", err)
	}
	defer r.Body.Close()

	body.Source = strings.TrimSpace(body.Source)
	body.Path = strings.TrimSpace(body.Path)
	if body.Source == "" || body.Path == "" || body.Sorting == nil {
		return http.StatusBadRequest, fmt.Errorf("source, path, and sorting are required")
	}

	switch body.Sorting.By {
	case "name", "size", "modified", "duration":
	default:
		return http.StatusBadRequest, fmt.Errorf("unsupported sorting field %q", body.Sorting.By)
	}

	source, ok := users.ResolveSourceKey(body.Source)
	if !ok {
		return http.StatusBadRequest, fmt.Errorf("source not found: %s", body.Source)
	}
	if !d.User.HasSourceByPath(source.Path) {
		return http.StatusForbidden, fmt.Errorf("access denied to source %s", body.Source)
	}

	u, err := state.GetUserByID(d.User.ID)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if u.DisplayPreferences == nil {
		u.DisplayPreferences = make(users.DisplayPreferences)
	}
	u.DisplayPreferences.SetSorting(body.Source, body.Path, *body.Sorting)
	if err := state.UpdateUser(&u, "", "DisplayPreferences"); err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusNoContent, nil
}
