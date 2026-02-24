package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"appcenter-agent/internal/ipc"
	"appcenter-agent/internal/tray"
)

func main() {
	if len(os.Args) == 1 {
		if err := tray.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "tray error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	action := "get_status"
	appID := 0

	action = strings.ToLower(os.Args[1])
	if action == "open_store" {
		if err := tray.OpenStoreUI(); err != nil {
			fmt.Fprintf(os.Stderr, "open_store error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if action == "open_store_legacy" {
		if err := tray.OpenStoreNativeUIStandalone(); err != nil {
			fmt.Fprintf(os.Stderr, "open_store_legacy error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if action == "check_server" {
		ok, detail := tray.CheckServerHealth()
		resp := map[string]any{"status": "ok", "server_reachable": ok, "detail": detail}
		b, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(b))
		if !ok {
			os.Exit(2)
		}
		return
	}
	if action == "install_from_store" && len(os.Args) > 2 {
		v, err := strconv.Atoi(os.Args[2])
		if err == nil {
			appID = v
		}
	}

	resp, err := ipc.SendRequest(ipc.NewRequest(action, appID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ipc error: %v\n", err)
		os.Exit(1)
	}

	b, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(b))
	if strings.ToLower(resp.Status) == "error" {
		os.Exit(2)
	}
}
