// Package version exposes the catraca build version.
package version

// Version is the catraca version, overridable at build time via
// -ldflags "-X github.com/davigiroux/catraca/internal/version.Version=v1.2.3".
var Version = "dev"
