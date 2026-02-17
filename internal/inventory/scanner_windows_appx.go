//go:build windows

package inventory

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type appxPkg struct {
	Name         string `json:"Name"`
	Version      string `json:"Version"`
	Publisher    string `json:"Publisher"`
	Architecture string `json:"Architecture"`
}

var guidLikeRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var hexLikeRe = regexp.MustCompile(`(?i)^[0-9a-f]{12,40}$`)

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
		rawName := strings.TrimSpace(p.Name)
		if rawName == "" {
			continue
		}
		// Skip non-user-facing packages that show up as GUID/hex identifiers.
		// These are typically Windows components that are not meaningful inventory entries.
		if guidLikeRe.MatchString(rawName) || hexLikeRe.MatchString(rawName) {
			continue
		}
		display := shortenAppxName(rawName)
		if display == "" || guidLikeRe.MatchString(display) || hexLikeRe.MatchString(display) {
			continue
		}
		items = append(items, SoftwareItem{
			Name:         display,
			Version:      strings.TrimSpace(p.Version),
			Publisher:    strings.TrimSpace(p.Publisher),
			Architecture: strings.TrimSpace(p.Architecture),
		})
	}
	return items
}

func shortenAppxName(pkgName string) string {
	pkgName = strings.TrimSpace(pkgName)
	if pkgName == "" {
		return ""
	}
	parts := strings.Split(pkgName, ".")
	if len(parts) == 0 {
		return pkgName
	}

	// If the package name ends with numeric components (e.g. Microsoft.WindowsAppRuntime.1.5),
	// returning only the last segment would be misleading ("5"). Prefer the last non-numeric
	// segment plus the numeric suffix: "WindowsAppRuntime 1.5".
	isDigits := func(s string) bool {
		if s == "" {
			return false
		}
		for i := 0; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		return true
	}

	// Collect numeric tail.
	tailStart := len(parts)
	for tailStart > 0 && isDigits(parts[tailStart-1]) {
		tailStart--
	}
	if tailStart == len(parts) {
		// No numeric suffix; keep the last segment (e.g. NanaZip).
		last := strings.TrimSpace(parts[len(parts)-1])
		if last == "" {
			return pkgName
		}
		return last
	}

	// Numeric suffix exists.
	suffix := strings.Join(parts[tailStart:], ".")
	base := ""
	if tailStart > 0 {
		base = strings.TrimSpace(parts[tailStart-1])
	}
	// Avoid overly generic base segments (e.g. "...WinAppRuntime.Main.1.5" -> "Main 1.5").
	// Prefer the previous segment when available.
	if (strings.EqualFold(base, "main") || strings.EqualFold(base, "manager")) && tailStart >= 2 {
		base = strings.TrimSpace(parts[tailStart-2])
	}
	if base == "" || suffix == "" {
		return pkgName
	}
	return base + " " + suffix
}
