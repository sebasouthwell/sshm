package cli

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/errors"
	"github.com/Sebasouthwell/sshm/internal/inventory"
	"github.com/Sebasouthwell/sshm/internal/provider"
	"github.com/Sebasouthwell/sshm/internal/token"
)

var testCmd = &cobra.Command{
	Use:   "test <alias>",
	Short: "Test connection to an entry",
	Long: `Test connection validation for an entry (provider-specific).

Examples:
  sshm test prod-web
  sshm test ssm-prod`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return HandleTest(args[0])
	},
}

// HandleTest handles the test command (exported for use from UI)
func HandleTest(alias string) error {
	entry, err := manager.Find(alias)
	if err != nil {
		return errors.NewNotFoundError(alias)
	}

	prov, err := GetProvider(entry.Type)
	if err != nil {
		return fmt.Errorf("failed to get provider: %s", entry.Type)
	}

	// Parse tokens (empty for test)
	parsed := token.Parse([]string{})
	opts := provider.RuntimeOpts{
		Tokens:      parsed.Tokens,
		Passthrough: parsed.Passthrough,
		Command:     parsed.Command,
	}

	// Test based on provider type
	switch entry.Type {
	case "ssh", "tf":
		return testSSH(entry, prov, opts)
	case "ssm":
		return testSSM(entry, prov, opts)
	case "docker":
		return testDocker(entry, prov, opts)
	case "kube":
		return testKube(entry, prov, opts)
	default:
		return fmt.Errorf("test not supported for provider type: %s", entry.Type)
	}
}

// testSSH tests SSH/TF connection
func testSSH(entry *inventory.Entry, prov provider.Provider, opts provider.RuntimeOpts) error {
	// Resolve entry
	resolved, err := prov.Resolve(entry, opts)
	if err != nil {
		return fmt.Errorf("failed to resolve entry: %w", err)
	}

	// Build test command (batchmode, timeout, no hostkey check)
	plan, err := prov.Build(provider.ActionOpen, entry, resolved, opts)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Modify command for test: add batchmode, timeout, no hostkey check
	testArgs := []string{}
	skipNext := false
	for i, arg := range plan.Argv {
		if skipNext {
			skipNext = false
			continue
		}

		if arg == "ssh" {
			testArgs = append(testArgs, arg)
			testArgs = append(testArgs, "-o", "ConnectTimeout=5")
			testArgs = append(testArgs, "-o", "BatchMode=yes")
			testArgs = append(testArgs, "-o", "StrictHostKeyChecking=no")
		} else if strings.HasPrefix(arg, "-i") || strings.HasPrefix(arg, "-p") {
			// Keep key/port option and its value
			testArgs = append(testArgs, arg)
			if i+1 < len(plan.Argv) {
				testArgs = append(testArgs, plan.Argv[i+1])
				skipNext = true
			}
		} else if !strings.HasPrefix(arg, "-") && i > 0 && arg != "ssh" {
			// This is the destination - add test command
			testArgs = append(testArgs, arg, "echo", "OK")
		} else {
			// Keep other args
			testArgs = append(testArgs, arg)
		}
	}

	fmt.Printf("Testing connection to %s...\n", resolved.Target)
	cmd := exec.Command(testArgs[0], testArgs[1:]...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Run with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return fmt.Errorf("connection failed (exit code: %d)", exitErr.ExitCode())
			}
			return fmt.Errorf("connection failed: %w", err)
		}
		fmt.Println("✓ Connection successful")
		return nil
	case <-time.After(6 * time.Second):
		cmd.Process.Kill()
		return fmt.Errorf("connection timeout")
	}
}

// testSSM tests SSM connection
func testSSM(entry *inventory.Entry, prov provider.Provider, opts provider.RuntimeOpts) error {
	// Verify AWS identity
	profile := entry.Meta["profile"]
	region := entry.Meta["region"]

	args := []string{"sts", "get-caller-identity"}
	if profile != "" {
		args = append([]string{"--profile", profile}, args...)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	fmt.Println("Verifying AWS identity...")
	cmd := exec.Command("aws", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("AWS identity verification failed: %w", err)
	}

	fmt.Println("✓ AWS identity verified")
	fmt.Printf("  %s\n", strings.TrimSpace(string(output)))

	// Optionally verify instance reachable
	if strings.HasPrefix(entry.Target, "i-") {
		fmt.Printf("Verifying instance %s is reachable...\n", entry.Target)
		describeArgs := []string{"ssm", "describe-instance-information", "--filters", "Key=InstanceIds,Values=" + entry.Target}
		if profile != "" {
			describeArgs = append([]string{"--profile", profile}, describeArgs...)
		}
		if region != "" {
			describeArgs = append(describeArgs, "--region", region)
		}

		describeCmd := exec.Command("aws", describeArgs...)
		if err := describeCmd.Run(); err != nil {
			return fmt.Errorf("instance not reachable via SSM: %w", err)
		}
		fmt.Println("✓ Instance is reachable via SSM")
	}

	return nil
}

// testDocker tests Docker connection
func testDocker(entry *inventory.Entry, prov provider.Provider, opts provider.RuntimeOpts) error {
	fmt.Printf("Testing Docker container: %s...\n", entry.Target)
	cmd := exec.Command("docker", "inspect", entry.Target)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("container not found or not running: %w", err)
	}

	// Check if container is running
	if !strings.Contains(string(output), `"Running":true`) {
		return fmt.Errorf("container exists but is not running")
	}

	fmt.Println("✓ Container is running")
	return nil
}

// testKube tests Kubernetes connection
func testKube(entry *inventory.Entry, prov provider.Provider, opts provider.RuntimeOpts) error {
	// Parse namespace/pod from target
	parts := strings.Split(entry.Target, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid kube target format (expected namespace/pod): %s", entry.Target)
	}

	namespace := parts[0]
	pod := parts[1]

	args := []string{"get", "pod", pod, "-n", namespace}
	if entry.Meta["context"] != "" {
		args = append([]string{"--context", entry.Meta["context"]}, args...)
	}

	fmt.Printf("Testing Kubernetes pod: %s/%s...\n", namespace, pod)
	cmd := exec.Command("kubectl", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("pod not found: %w", err)
	}

	// Check if pod is ready
	if !strings.Contains(string(output), "Running") && !strings.Contains(string(output), "Ready") {
		return fmt.Errorf("pod exists but is not ready")
	}

	fmt.Println("✓ Pod is ready")
	return nil
}
