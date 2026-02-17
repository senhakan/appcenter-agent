//go:build windows

package inventory

import (
	"fmt"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

// registryPaths lists the Uninstall registry locations to scan.
var registryPaths = []struct {
	Root registry.Key
	Path string
}{
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
}

// ScanInstalledSoftware reads the Windows registry and returns all installed software items.
func ScanInstalledSoftware() []SoftwareItem {
	seen := make(map[string]bool)
	var items []SoftwareItem

	for _, rp := range registryPaths {
		key, err := registry.OpenKey(rp.Root, rp.Path, registry.READ)
		if err != nil {
			continue
		}
		subKeys, err := key.ReadSubKeyNames(-1)
		key.Close()
		if err != nil {
			continue
		}
		for _, sub := range subKeys {
			subKey, err := registry.OpenKey(rp.Root, rp.Path+`\`+sub, registry.READ)
			if err != nil {
				continue
			}
			item := readSoftwareItem(subKey)
			subKey.Close()

			if item.Name == "" {
				continue
			}
			// Deduplicate by name+version
			dedup := item.Name + "|" + item.Version
			if seen[dedup] {
				continue
			}
			seen[dedup] = true

			// Skip system components and updates
			sysComp, _, _ := subKey.GetIntegerValue("SystemComponent")
			if sysComp == 1 {
				continue
			}

			items = append(items, item)
		}
	}
	return items
}

func readSoftwareItem(key registry.Key) SoftwareItem {
	name, _, _ := key.GetStringValue("DisplayName")
	version, _, _ := key.GetStringValue("DisplayVersion")
	publisher, _, _ := key.GetStringValue("Publisher")
	installDate, _, _ := key.GetStringValue("InstallDate")
	sizeVal, _, err := key.GetIntegerValue("EstimatedSize")
	sizeKB := 0
	if err == nil {
		sizeKB = int(sizeVal)
	}

	// Try string fallback for EstimatedSize (some apps store it as REG_SZ)
	if sizeKB == 0 {
		if sizeStr, _, err := key.GetStringValue("EstimatedSize"); err == nil {
			if v, err := strconv.Atoi(sizeStr); err == nil {
				sizeKB = v
			}
		}
	}

	arch := ""
	if name != "" {
		// Infer architecture from registry path context — caller can refine
		arch = inferArchitecture(key)
	}

	return SoftwareItem{
		Name:            name,
		Version:         version,
		Publisher:        publisher,
		InstallDate:     installDate,
		EstimatedSizeKB: sizeKB,
		Architecture:    arch,
	}
}

func inferArchitecture(key registry.Key) string {
	// Best-effort: check DisplayName for hints
	name, _, _ := key.GetStringValue("DisplayName")
	for _, hint := range []string{"(x64)", "64-bit", "x64", "amd64"} {
		if containsCI(name, hint) {
			return "x64"
		}
	}
	for _, hint := range []string{"(x86)", "32-bit", "x86"} {
		if containsCI(name, hint) {
			return "x86"
		}
	}
	return ""
}

func containsCI(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			a, b := s[i+j], substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func init() {
	_ = fmt.Sprintf // ensure fmt imported
}
