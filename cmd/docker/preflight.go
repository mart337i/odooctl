package docker

import (
	"time"

	"github.com/mart337i/odooctl/internal/config"
	dockerlib "github.com/mart337i/odooctl/internal/docker"
)

func ensureDockerProjectAccess(state *config.State) error {
	var daemonErr error
	for attempt := 0; attempt < 3; attempt++ {
		daemonErr = dockerlib.CheckDaemon()
		if daemonErr == nil {
			break
		}
		if !dockerlib.IsRetryable(daemonErr) || attempt == 2 {
			return daemonErr
		}
		time.Sleep(time.Duration(1<<attempt) * 500 * time.Millisecond)
	}
	if err := dockerlib.CheckCompose(); err != nil {
		return err
	}
	if err := dockerlib.CheckComposeConfig(state); err != nil {
		return err
	}

	var bindErr error
	for attempt := 0; attempt < 2; attempt++ {
		bindErr = dockerlib.CheckBindMount(state.ProjectRoot)
		if bindErr == nil || !dockerlib.IsRetryable(bindErr) || attempt == 1 {
			return bindErr
		}
		time.Sleep(500 * time.Millisecond)
	}
	return bindErr
}
