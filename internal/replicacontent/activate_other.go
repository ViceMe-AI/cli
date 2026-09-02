//go:build !darwin && !linux && !windows

package replicacontent

func atomicInstallSupported() bool { return false }
