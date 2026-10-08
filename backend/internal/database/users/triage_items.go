package users

import "strings"

const (
	TriageStatusKeep   = "keep"
	TriageStatusReject = "reject"
	TriageStatusMaybe  = "maybe"
)

// TriageItems stores a per-user review state for files and folders.
// The keys are source filesystem path -> index directory path -> item basename -> status.
type TriageItems map[string]map[string]map[string]string

// NormalizeTriageStatus validates and normalizes a triage state.
// An empty string clears an existing state.
func NormalizeTriageStatus(status string) (string, bool) {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "none", "unmarked":
		return "", true
	case TriageStatusKeep, TriageStatusReject, TriageStatusMaybe:
		return status, true
	default:
		return "", false
	}
}

// Set records status for an item. An empty status removes the item and prunes empty maps.
func (t TriageItems) Set(sourcePath, indexDirPath, name, status string) {
	if t == nil {
		return
	}
	if status == "" {
		t.Remove(sourcePath, indexDirPath, name)
		return
	}
	if t[sourcePath] == nil {
		t[sourcePath] = make(map[string]map[string]string)
	}
	if t[sourcePath][indexDirPath] == nil {
		t[sourcePath][indexDirPath] = make(map[string]string)
	}
	t[sourcePath][indexDirPath][name] = status
}

// Remove clears an item's triage state and prunes empty maps.
func (t TriageItems) Remove(sourcePath, indexDirPath, name string) {
	if t == nil {
		return
	}
	byDir, ok := t[sourcePath]
	if !ok {
		return
	}
	byName, ok := byDir[indexDirPath]
	if !ok {
		return
	}
	delete(byName, name)
	if len(byName) == 0 {
		delete(byDir, indexDirPath)
	}
	if len(byDir) == 0 {
		delete(t, sourcePath)
	}
}

// ForDirectory returns a copy of the item-name -> status map for one directory.
func (u *User) TriageItemsForDirectory(sourcePath, indexDirPath string) map[string]string {
	if u == nil || len(u.TriageItems) == 0 {
		return nil
	}
	byDir, ok := u.TriageItems[sourcePath]
	if !ok {
		return nil
	}
	byName, ok := byDir[indexDirPath]
	if !ok || len(byName) == 0 {
		return nil
	}
	out := make(map[string]string, len(byName))
	for name, status := range byName {
		out[name] = status
	}
	return out
}

// TriageStatusForItem returns the state of one item, or an empty string when unmarked.
func (u *User) TriageStatusForItem(sourcePath, indexDirPath, name string) string {
	if u == nil || len(u.TriageItems) == 0 {
		return ""
	}
	return u.TriageItems[sourcePath][indexDirPath][name]
}

// EnsureTriageItems returns a non-nil map for mutation.
func (u *User) EnsureTriageItems() TriageItems {
	if u.TriageItems == nil {
		u.TriageItems = make(TriageItems)
	}
	return u.TriageItems
}
