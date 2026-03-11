package system

import "testing"

func TestNormalizeSessionType(t *testing.T) {
	if got := normalizeSessionType(0); got != "local" {
		t.Fatalf("normalizeSessionType(0) = %q, want %q", got, "local")
	}
	if got := normalizeSessionType(2); got != "rdp" {
		t.Fatalf("normalizeSessionType(2) = %q, want %q", got, "rdp")
	}
}

func TestNormalizeSessionState(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"active", sessionStateActive},
		{"Active", sessionStateActive},
		{"disc", sessionStateDisconnected},
		{"disconnected", sessionStateDisconnected},
		{"Disc", sessionStateDisconnected},
		{"", sessionStateActive},
	}
	for _, tc := range cases {
		if got := normalizeSessionState(tc.in); got != tc.want {
			t.Fatalf("normalizeSessionState(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

