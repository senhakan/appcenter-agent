package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"appcenter-agent/internal/ipc"
)

func main() {
	action := "get_status"
	appID := 0

	if len(os.Args) > 1 {
		action = strings.ToLower(os.Args[1])
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
