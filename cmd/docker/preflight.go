package docker

import (
	"github.com/mart337i/odooctl/internal/config"
	dockerlib "github.com/mart337i/odooctl/internal/docker"
)

func ensureDockerProjectAccess(state *config.State) error {
	if err := dockerlib.CheckDaemon(); err != nil {
		return err
	}
	return dockerlib.CheckBindMount(state.ProjectRoot)
}
