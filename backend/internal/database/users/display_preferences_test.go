package users

import "testing"

func TestDisplayPreferencesSetSortingIsDirectoryScoped(t *testing.T) {
	prefs := make(DisplayPreferences)
	prefs.SetSorting("Media", "/files/Media/", Sorting{By: "modified", Asc: false})
	prefs.SetSorting("Media", "/files/Media/comic-a/", Sorting{By: "name", Asc: true})

	root := prefs["Media"]["/files/Media/"].Sorting
	child := prefs["Media"]["/files/Media/comic-a/"].Sorting
	if root == nil || root.By != "modified" || root.Asc {
		t.Fatalf("unexpected root sorting: %#v", root)
	}
	if child == nil || child.By != "name" || !child.Asc {
		t.Fatalf("unexpected child sorting: %#v", child)
	}
}

func TestDisplayPreferencesSetSortingReplacesOnlyTargetDirectory(t *testing.T) {
	prefs := make(DisplayPreferences)
	prefs.SetSorting("Media", "/parent/", Sorting{By: "modified", Asc: false})
	prefs.SetSorting("Media", "/parent/child/", Sorting{By: "name", Asc: true})
	prefs.SetSorting("Media", "/parent/child/", Sorting{By: "size", Asc: false})

	parent := prefs["Media"]["/parent/"].Sorting
	child := prefs["Media"]["/parent/child/"].Sorting
	if parent == nil || parent.By != "modified" {
		t.Fatalf("parent preference changed unexpectedly: %#v", parent)
	}
	if child == nil || child.By != "size" || child.Asc {
		t.Fatalf("child preference was not replaced: %#v", child)
	}
}
