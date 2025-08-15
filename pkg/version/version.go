package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is set via ldflags during build
	Version = "dev"
	// GitCommit is set via ldflags during build
	GitCommit = "unknown"
	// BuildDate is set via ldflags during build
	BuildDate = "unknown"
)

// GetVersion returns the full version string
func GetVersion() string {
	return fmt.Sprintf("Version: %s\nGit Commit: %s\nBuild Date: %s\nGo Version: %s\nOS/Arch: %s/%s",
		Version,
		GitCommit,
		BuildDate,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)
}

// GetShortVersion returns just the version number
func GetShortVersion() string {
	return Version
}