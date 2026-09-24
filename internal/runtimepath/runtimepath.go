// Package runtimepath resolves per-user runtime paths used by Voxi services.
package runtimepath

import (
	"fmt"
	"path/filepath"
)

// UserDir returns XDG_RUNTIME_DIR or the conventional systemd user runtime path.
func UserDir(xdgRuntimeDir string, uid int) string {
	if xdgRuntimeDir != "" {
		return xdgRuntimeDir
	}
	return filepath.Join("/run/user", fmt.Sprint(uid))
}

// VoxiDir returns Voxi's private runtime directory below the per-user runtime dir.
func VoxiDir(xdgRuntimeDir string, uid int) string {
	return filepath.Join(UserDir(xdgRuntimeDir, uid), "voxi")
}
