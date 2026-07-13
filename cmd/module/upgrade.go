package module

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade <module...>",
	Short: "Install or update modules through docker install",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runUpgrade,
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	upgradeArgs := append([]string{"docker", "install"}, args...)
	child := exec.Command(executable, upgradeArgs...)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Stdin = os.Stdin
	return child.Run()
}
