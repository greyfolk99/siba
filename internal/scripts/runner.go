package scripts

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/greyfolk99/siba/internal/workspace"
)

// RunScript runs a named script from [scripts] section
func RunScript(name string, config *workspace.ModuleConfig) error {
	if config == nil || config.Scripts == nil {
		return fmt.Errorf("no scripts defined")
	}

	cmd, ok := config.Scripts[name]
	if !ok {
		return fmt.Errorf("script %q not found", name)
	}

	return executeCommand(cmd)
}

// RunPreexport runs the preexport script if defined
func RunPreexport(config *workspace.ModuleConfig) error {
	if config == nil || config.Scripts == nil {
		return nil
	}
	if cmd, ok := config.Scripts["preexport"]; ok {
		fmt.Println("running preexport...")
		return executeCommand(cmd)
	}
	return nil
}

// RunPostexport runs the postexport script if defined
func RunPostexport(config *workspace.ModuleConfig) error {
	if config == nil || config.Scripts == nil {
		return nil
	}
	if cmd, ok := config.Scripts["postexport"]; ok {
		fmt.Println("running postexport...")
		return executeCommand(cmd)
	}
	return nil
}

func executeCommand(cmdStr string) error {
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
