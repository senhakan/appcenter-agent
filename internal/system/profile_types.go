package system

// SystemDisk captures disk details for system profile reporting.
type SystemDisk struct {
	Index   int    `json:"index"`
	SizeGB  int    `json:"size_gb,omitempty"`
	Model   string `json:"model,omitempty"`
	BusType string `json:"bus_type,omitempty"`
}

type VirtualizationInfo struct {
	IsVirtual bool   `json:"is_virtual"`
	Vendor    string `json:"vendor,omitempty"`
	Model     string `json:"model,omitempty"`
}

// SystemProfile represents relatively static machine information that should
// be reported periodically (not on every heartbeat).
type SystemProfile struct {
	OSFullName   string `json:"os_full_name,omitempty"`
	OSVersion    string `json:"os_version,omitempty"`
	BuildNumber  string `json:"build_number,omitempty"`
	Architecture string `json:"architecture,omitempty"`

	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`

	CPUModel         string `json:"cpu_model,omitempty"`
	CPUCoresPhysical int    `json:"cpu_cores_physical,omitempty"`
	CPUCoresLogical  int    `json:"cpu_cores_logical,omitempty"`

	TotalMemoryGB int `json:"total_memory_gb,omitempty"`

	DiskCount int          `json:"disk_count,omitempty"`
	Disks     []SystemDisk `json:"disks,omitempty"`

	Virtualization *VirtualizationInfo `json:"virtualization,omitempty"`
}

