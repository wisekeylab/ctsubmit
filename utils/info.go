package utils

import (
	"runtime/debug"
	"strings"
)

const NOT_INSTALLED = "not installed"

func GetPackagePath() string {
	// Extract the package's repository URL from the build info embedded into the executable.
	if bi, ok := debug.ReadBuildInfo(); ok {
		return bi.Path
	}
	return ""
}

func VersionString(version string) string {
	if version == NOT_INSTALLED {
		return "[" + version + "]"
	} else if strings.Contains(version, "-g") {
		// git describe format: v0.0.0-0-gabcdef1
		return version
	} else if idx := strings.LastIndex(version, "-"); idx != -1 && len(version)-idx > 7 {
		// go.mod version format: v0.0.0-20210101000000-abcdef123456
		return version
	} else if strings.Contains(version, ".") {
		// Stable version format: v0.0.0
		if !strings.HasPrefix(version, "v") {
			version = "v" + version
		}
		return version
	} else if len(version) >= 7 {
		// Git commit hash format: abcdef123456...
		return "g" + version[0:7]
	} else {
		return "(" + version + ")"
	}
}
