package git

import "testing"

func TestParseStatusPorcelain(t *testing.T) {
	raw := "?? new.txt\n M modified.txt\nD  deleted.txt\nR  old.txt -> renamed.txt\nUU conflict.txt\n!! ignored.txt\n"
	got := ParseStatusPorcelain(raw)
	want := []FileStatus{
		{Code: "??", Type: "added", Path: "new.txt"},
		{Code: " M", Type: "modified", Path: "modified.txt"},
		{Code: "D ", Type: "deleted", Path: "deleted.txt"},
		{Code: "R ", Type: "renamed", Path: "renamed.txt"},
		{Code: "UU", Type: "conflict", Path: "conflict.txt"},
		{Code: "!!", Type: "other", Path: "ignored.txt"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}
