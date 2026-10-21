package update

import (
	"os"
	"strings"
)

// Install types DetectInstall tells apart.
const (
	InstallUnraid = "unraid"
	InstallDocker = "docker"
	InstallBinary = "binary"
)

// DetectInstall is how this server was installed. Unraid gives each
// container HOST_OS=Unraid; Docker leaves /.dockerenv, and Podman sets
// container. Anything else runs the file from a release (a build from source
// shows no update notice).
func DetectInstall(getenv func(string) string, exists func(string) bool) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(getenv("HOST_OS")), "unraid"):
		return InstallUnraid
	case exists("/.dockerenv") || exists("/run/.containerenv") || getenv("container") != "":
		return InstallDocker
	default:
		return InstallBinary
	}
}

// HowTo is how to update an install of this type. Empty for an unknown type.
func HowTo(install string) string {
	switch install {
	case InstallUnraid:
		return "In Unraid, open Docker, choose Check for Updates, then apply the update for Broadwave. The Broadwave updater template does this at night."
	case InstallDocker:
		return "Pull the new image and recreate the container with the same folders: docker compose pull && docker compose up -d, or docker pull ghcr.io/wolfebase/broadwave:latest and run it again."
	case InstallBinary:
		return "Download the file for this computer from the release page, stop the server, replace the file, and start it again."
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
