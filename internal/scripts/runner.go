package scripts

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/hjseo/siba/internal/workspace"
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

// RunPrerender runs the prerender script if defined
func RunPrerender(config *workspace.ModuleConfig) error {
	if config == nil || config.Scripts == nil {
		return nil
	}
	if cmd, ok := config.Scripts["prerender"]; ok {
		fmt.Println("running prerender...")
		return executeCommand(cmd)
	}
	return nil
}

// RunPostrender runs the postrender script if defined
func RunPostrender(config *workspace.ModuleConfig) error {
	if config == nil || config.Scripts == nil {
		return nil
	}
	if cmd, ok := config.Scripts["postrender"]; ok {
		fmt.Println("running postrender...")
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
