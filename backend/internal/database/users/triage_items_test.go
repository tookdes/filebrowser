package users

import "testing"

func TestTriageItemsSetReplaceClear(t *testing.T) {
	items := make(TriageItems)
	items.Set("/srv/media", "/comics/", "book-a", TriageStatusKeep)

	if got := items["/srv/media"]["/comics/"]["book-a"]; got != TriageStatusKeep {
		t.Fatalf("expected keep, got %q", got)
	}

	items.Set("/srv/media", "/comics/", "book-a", TriageStatusReject)
	if got := items["/srv/media"]["/comics/"]["book-a"]; got != TriageStatusReject {
		t.Fatalf("expected reject, got %q", got)
	}

	items.Set("/srv/media", "/comics/", "book-a", "")
	if len(items) != 0 {
		t.Fatalf("expected empty map after clear, got %#v", items)
	}
}

func TestTriageItemsForDirectoryReturnsCopy(t *testing.T) {
	u := &User{TriageItems: make(TriageItems)}
	u.TriageItems.Set("/srv/media", "/video/", "a.mp4", TriageStatusReject)

	got := u.TriageItemsForDirectory("/srv/media", "/video/")
	got["a.mp4"] = TriageStatusKeep

	if status := u.TriageStatusForItem("/srv/media", "/video/", "a.mp4"); status != TriageStatusReject {
		t.Fatalf("mutating returned map changed stored value: %q", status)
	}
}

func TestLegacyMaybeStatusIsTreatedAsUnmarked(t *testing.T) {
	u := &User{TriageItems: make(TriageItems)}
	u.TriageItems.Set("/srv/media", "/video/", "a.mp4", "maybe")

	if got := u.TriageItemsForDirectory("/srv/media", "/video/"); got != nil {
		t.Fatalf("expected legacy maybe status to be hidden, got %#v", got)
	}
	if status := u.TriageStatusForItem("/srv/media", "/video/", "a.mp4"); status != "" {
		t.Fatalf("expected legacy maybe status to read as unmarked, got %q", status)
	}
}

func TestNormalizeTriageStatus(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"KEEP", TriageStatusKeep, true},
		{" reject ", TriageStatusReject, true},
		{"maybe", "", false},
		{"unmarked", "", true},
		{"", "", true},
		{"later", "", false},
	}

	for _, tc := range tests {
		got, ok := NormalizeTriageStatus(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("NormalizeTriageStatus(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
