//go:build windows

package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type psSystemProfile struct {
	OSFullName   string `json:"os_full_name"`
	OSVersion    string `json:"os_version"`
	BuildNumber  string `json:"build_number"`
	Architecture string `json:"architecture"`

	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`

	CPUModel         string `json:"cpu_model"`
	CPUCoresPhysical int    `json:"cpu_cores_physical"`
	CPUCoresLogical  int    `json:"cpu_cores_logical"`

	TotalMemoryGB int `json:"total_memory_gb"`

	DiskCount int          `json:"disk_count"`
	Disks     []SystemDisk `json:"disks"`

	Virtualization *VirtualizationInfo `json:"virtualization"`
}

func CollectSystemProfile() (*SystemProfile, error) {
	ps := strings.Join([]string{
		"$ErrorActionPreference='SilentlyContinue';",
		"$os = Get-CimInstance Win32_OperatingSystem | Select-Object Caption, Version, BuildNumber, OSArchitecture;",
		"$cs = Get-CimInstance Win32_ComputerSystem | Select-Object Manufacturer, Model, TotalPhysicalMemory;",
		"$cpu = Get-CimInstance Win32_Processor | Select-Object -First 1 Name, NumberOfCores, NumberOfLogicalProcessors;",
		"$diskDrives = Get-CimInstance Win32_DiskDrive | Select-Object Index, Model, Size;",
		"$disks = @();",
		"try { $gd = Get-Disk | Select-Object Number, BusType; } catch { $gd = @(); }",
		"foreach($d in $diskDrives){",
		"  $bus='';",
		"  $g = $gd | Where-Object { $_.Number -eq $d.Index } | Select-Object -First 1;",
		"  if($g){ $bus = [string]$g.BusType }",
		"  $disks += [pscustomobject]@{ index=[int]$d.Index; size_gb=[int]([math]::Round(($d.Size/1GB),0)); model=[string]$d.Model; bus_type=$bus };",
		"}",
		"$m=[string]$cs.Model; $man=[string]$cs.Manufacturer;",
		"$isVirt=$false; $virtVendor=''; $virtModel='';",
		"$virtHints=@('vmware','virtualbox','kvm','hyper-v','xen','virtual');",
		"$check=($man+' '+$m).ToLower();",
		"foreach($h in $virtHints){ if($check -like ('*'+$h+'*')){ $isVirt=$true } }",
		"if($isVirt){ $virtVendor=$man; $virtModel=$m }",
		"$out=[pscustomobject]@{",
		" os_full_name=[string]$os.Caption;",
		" os_version=[string]$os.Version;",
		" build_number=[string]$os.BuildNumber;",
		" architecture=[string]$os.OSArchitecture;",
		" manufacturer=$man;",
		" model=$m;",
		" cpu_model=[string]$cpu.Name;",
		" cpu_cores_physical=[int]$cpu.NumberOfCores;",
		" cpu_cores_logical=[int]$cpu.NumberOfLogicalProcessors;",
		" total_memory_gb=[int]([math]::Round(($cs.TotalPhysicalMemory/1GB),0));",
		" disk_count=[int]$disks.Count;",
		" disks=$disks;",
		" virtualization=[pscustomobject]@{ is_virtual=$isVirt; vendor=$virtVendor; model=$virtModel }",
		"};",
		"$out | ConvertTo-Json -Compress",
	}, " ")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps).Output()
	if err != nil || len(out) == 0 {
		return nil, fmt.Errorf("collect system profile failed: %w", err)
	}

	var raw psSystemProfile
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}

	// Keep disk list order stable for hashing/history readability.
	if len(raw.Disks) > 1 {
		sort.Slice(raw.Disks, func(i, j int) bool { return raw.Disks[i].Index < raw.Disks[j].Index })
	}

	p := &SystemProfile{
		OSFullName:   strings.TrimSpace(raw.OSFullName),
		OSVersion:    strings.TrimSpace(raw.OSVersion),
		BuildNumber:  strings.TrimSpace(raw.BuildNumber),
		Architecture: strings.TrimSpace(raw.Architecture),
		Manufacturer: strings.TrimSpace(raw.Manufacturer),
		Model:        strings.TrimSpace(raw.Model),
		CPUModel:     strings.TrimSpace(raw.CPUModel),
		CPUCoresPhysical: raw.CPUCoresPhysical,
		CPUCoresLogical:  raw.CPUCoresLogical,
		TotalMemoryGB:    raw.TotalMemoryGB,
		DiskCount:        raw.DiskCount,
		Disks:            raw.Disks,
		Virtualization:   raw.Virtualization,
	}
	return p, nil
}

