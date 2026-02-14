package tray

import "testing"

func TestStatusTooltip(t *testing.T) {
	s := StatusSnapshot{Service: "running", PendingTasks: 3}
	got := statusTooltip(s)
	want := "AppCenter Agent - running (pending: 3)"
	if got != want {
		t.Fatalf("tooltip=%q want=%q", got, want)
	}
}

func TestAppLabel(t *testing.T) {
	app := StoreApp{ID: 7, DisplayName: "7-Zip", Version: "23.01", Installed: true}
	got := appLabel(app)
	want := "7 - 7-Zip 23.01 [installed]"
	if got != want {
		t.Fatalf("label=%q want=%q", got, want)
	}
}
