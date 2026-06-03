package utils

import "runtime/debug"

func GetPackagePath() string {
	// Extract the package's repository URL from the build info embedded into the executable.
	if bi, ok := debug.ReadBuildInfo(); ok {
		return bi.Path
	}
	return ""
}
