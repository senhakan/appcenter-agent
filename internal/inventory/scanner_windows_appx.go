//go:build windows

package inventory

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

type appxPkg struct {
	Name         string `json:"Name"`
	Version      string `json:"Version"`
	Publisher    string `json:"Publisher"`
	Architecture string `json:"Architecture"`
}

func scanAppxPackagesAllUsers() []SoftwareItem {
	// PowerShell is the most pragmatic way to enumerate Appx packages without
	// pulling in WinRT bindings. Keep it time-bounded.
	//
	// Note: DisplayName may be a resource key; we use package Name and apply a
	// best-effort shortening (e.g. 40174MouriNaruto.NanaZip -> NanaZip).
	ps := strings.Join([]string{
		"$ErrorActionPreference='SilentlyContinue';",
		"Get-AppxPackage -AllUsers",
		"| Where-Object { $_.Name -and (-not $_.IsFramework) -and (-not $_.IsResourcePackage) }",
		"| Select-Object",
		"@{n='Name';e={$_.Name}},",
		"@{n='Version';e={$_.Version.ToString()}},",
		"@{n='Publisher';e={$_.Publisher}},",
		"@{n='Architecture';e={$_.Architecture.ToString()}}",
		"| ConvertTo-Json -Compress",
	}, " ")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps).Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	// ConvertTo-Json returns either an object or an array depending on count.
	var many []appxPkg
	if err := json.Unmarshal(out, &many); err != nil {
		var one appxPkg
		if err2 := json.Unmarshal(out, &one); err2 != nil {
			return nil
		}
		many = []appxPkg{one}
	}

	items := make([]SoftwareItem, 0, len(many))
	for _, p := range many {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		items = append(items, SoftwareItem{
			Name:         shortenAppxName(name),
			Version:      strings.TrimSpace(p.Version),
			Publisher:    strings.TrimSpace(p.Publisher),
			Architecture: strings.TrimSpace(p.Architecture),
		})
	}
	return items
}

func shortenAppxName(pkgName string) string {
	parts := strings.Split(pkgName, ".")
	if len(parts) == 0 {
		return pkgName
	}
	last := parts[len(parts)-1]
	last = strings.TrimSpace(last)
	if last == "" {
		return pkgName
	}
	return last
}
