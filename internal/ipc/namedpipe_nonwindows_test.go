//go:build !windows

package ipc

import "testing"

func TestNonWindowsReturnsError(t *testing.T) {
	if _, err := StartPipeServer(func(Request) Response { return Response{Status: "ok"} }); err == nil {
		t.Fatal("expected error on non-windows")
	}
	if _, err := SendRequest(NewRequest("get_status", 0)); err == nil {
		t.Fatal("expected error on non-windows")
	}
}
