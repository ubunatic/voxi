package voxi

import "embed"

//go:embed systemd/*.service
var serviceAssets embed.FS

// ServiceAsset returns a packaged systemd unit from the repository's canonical
// systemd directory. Installers and Make targets therefore share one unit.
func ServiceAsset(name string) ([]byte, error) {
	return serviceAssets.ReadFile("systemd/" + name)
}
