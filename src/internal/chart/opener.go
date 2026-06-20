package chart

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openFile opens a file with the default system application.
func openFile(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		// Wrap path in quotes to prevent path injection via special characters
		cmd = exec.Command("cmd", "/c", "start", "", "\""+path+"\"")
	case "linux":
		cmd = exec.Command("xdg-open", path)
	default:
		return fmt.Errorf("자동 열기를 지원하지 않는 OS입니다: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait in a goroutine to prevent zombie processes
	go cmd.Wait()

	return nil
}
