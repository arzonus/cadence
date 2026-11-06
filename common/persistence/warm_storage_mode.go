package persistence

// WarmStorageMode is the per-domain value of system.warmStorageMode.
type WarmStorageMode string

const (
	// WarmStorageOff never touches closed_executions.
	WarmStorageOff WarmStorageMode = "off"
	// WarmStorageShadow writes slim rows on close and compares them on read, but keeps every executions row.
	WarmStorageShadow WarmStorageMode = "shadow"
	// WarmStorageReadOnly moves nothing new but still reads, restores and deletes slim rows.
	WarmStorageReadOnly WarmStorageMode = "read_only"
	// WarmStorageOn moves closed runs out of executions.
	WarmStorageOn WarmStorageMode = "on"
)

// ParseWarmStorageMode treats an unknown value as off, the safe side: no contact with closed_executions.
func ParseWarmStorageMode(s string) WarmStorageMode {
	switch m := WarmStorageMode(s); m {
	case WarmStorageShadow, WarmStorageReadOnly, WarmStorageOn:
		return m
	}
	return WarmStorageOff
}

// MovesRuns reports whether the close task should call MoveToClosedExecutions.
func (m WarmStorageMode) MovesRuns() bool {
	return m == WarmStorageShadow || m == WarmStorageOn
}
