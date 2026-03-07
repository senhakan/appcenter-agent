package system

// ServiceInfo is a cross-platform service snapshot item.
type ServiceInfo struct {
	Name        string
	DisplayName string
	Status      string
	StartupType string
	PID         int
	RunAs       string
	Description string
}
