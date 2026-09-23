package config

import (
	"os"
	"path/filepath"
)

// PolyDir is the directory that holds config.toml and related local state.
func PolyDir(projectRoot, polyPath string) string {
	if filepath.IsAbs(polyPath) {
		return polyPath
	}
	return filepath.Join(projectRoot, polyPath)
}

// ProjectConfigFile is {polyDir}/config.toml.
func ProjectConfigFile(projectRoot, polyPath string) string {
	return filepath.Join(PolyDir(projectRoot, polyPath), "config.toml")
}

// UserConfigDir is $XDG_CONFIG_HOME/poly or ~/.config/poly (XDG even on macOS).
func UserConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "poly")
	}
	return filepath.Join(homeDir(), ".config", "poly")
}

// UserConfigFile is the user-level config.toml.
func UserConfigFile() string {
	return filepath.Join(UserConfigDir(), "config.toml")
}

// DeviceKeyFile is the file-backed device secret (keychain fallback).
func DeviceKeyFile(userConfigDir string) string {
	return filepath.Join(userConfigDir, "device.key")
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		return profile
	}
	return "."
}
