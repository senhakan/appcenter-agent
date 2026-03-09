//go:build windows

package main

import (
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func ensureRemoteSupportFirewallRules(exeDir string, logger *log.Logger) {
	helperPath := filepath.Join(exeDir, "rshelper.exe")
	ensureRule := func(name string, port int) {
		showOut, showErr := exec.Command(
			"netsh",
			"advfirewall",
			"firewall",
			"show",
			"rule",
			"name="+name,
		).CombinedOutput()
		if showErr == nil && !strings.Contains(strings.ToLower(string(showOut)), "no rules match") {
			return
		}
		addOut, addErr := exec.Command(
			"netsh",
			"advfirewall",
			"firewall",
			"add",
			"rule",
			"name="+name,
			"dir=in",
			"action=allow",
			"enable=yes",
			"profile=any",
			"protocol=TCP",
			"localport="+strconv.Itoa(port),
			"program="+helperPath,
		).CombinedOutput()
		if addErr != nil {
			logger.Printf("remote support firewall rule add failed: %s port=%d err=%v out=%s", name, port, addErr, strings.TrimSpace(string(addOut)))
			return
		}
		logger.Printf("remote support firewall rule ensured: %s port=%d", name, port)
	}
	ensureRule("AppCenter RemoteSupport 20010", 20010)
	ensureRule("AppCenter RemoteSupport 20011", 20011)
}
