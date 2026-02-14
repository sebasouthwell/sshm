package utils

import (
	"fmt"
	"io"
	"os/exec"
)

// CopyToClipboard copies text to the system clipboard
// Supports: macOS (pbcopy), Linux X11 (xclip), Linux Wayland (wl-copy), Windows (clip.exe)
// Returns error if no clipboard tool is available
func CopyToClipboard(text string) error {
	// Try macOS
	if _, err := exec.LookPath("pbcopy"); err == nil {
		return copyViaStdin("pbcopy", text)
	}

	// Try Wayland
	if _, err := exec.LookPath("wl-copy"); err == nil {
		return copyViaStdin("wl-copy", text)
	}

	// Try X11
	if _, err := exec.LookPath("xclip"); err == nil {
		return copyViaStdinWithFlags("xclip", []string{"-selection", "clipboard"}, text)
	}

	// Try Windows
	if _, err := exec.LookPath("clip.exe"); err == nil {
		return copyViaStdin("clip.exe", text)
	}

	return fmt.Errorf("no clipboard tool found (tried: pbcopy, wl-copy, xclip, clip.exe)")
}

// copyViaStdin copies text via a command that reads from stdin
func copyViaStdin(cmdName string, text string) error {
	cmd := exec.Command(cmdName)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	_, err = io.WriteString(stdin, text)
	if err != nil {
		stdin.Close()
		cmd.Wait()
		return err
	}
	stdin.Close()

	return cmd.Wait()
}

// copyViaStdinWithFlags copies text via a command with flags that reads from stdin
func copyViaStdinWithFlags(cmdName string, flags []string, text string) error {
	args := append(flags)
	cmd := exec.Command(cmdName, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	_, err = io.WriteString(stdin, text)
	if err != nil {
		stdin.Close()
		cmd.Wait()
		return err
	}
	stdin.Close()

	return cmd.Wait()
}
