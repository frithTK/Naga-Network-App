// Package version holds the naga-control semver used by health and release builds.
package version

// Version is the control-plane semver without a leading "v".
// Release builds override it with:
//
//	-ldflags "-X naga.network/core/version.Version=X.Y.Z"
var Version = "dev"
