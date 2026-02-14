package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Sebasouthwell/sshm/internal/inventory"
)

var (
	tfListDetails bool
	tfListFZF     bool
)

var tfWizardCmd = &cobra.Command{
	Use:   "tf",
	Short: "Terraform integration",
	Long: `Terraform integration for SSHM.

Commands:
  sshm tf                    Interactive add wizard
  sshm tf list [--details]   List terraform resources
  sshm tf add [args...]      Add terraform entry (interactive if args missing)
  
If first argument is an alias, opens connection (provider convenience):
  sshm tf <alias>             Open connection to terraform entry`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If first arg looks like an alias (not a subcommand), route to provider command
		if len(args) > 0 {
			firstArg := args[0]
			// Check if it's a known alias (not a subcommand)
			if firstArg != "list" && firstArg != "add" && !strings.HasPrefix(firstArg, "-") {
				if entry, err := manager.Find(firstArg); err == nil && entry.Type == "tf" {
					// It's an alias, route to provider command
					return handleProviderCommand("tf", firstArg, args[1:])
				}
			}
		}
		// Default to interactive wizard if no subcommand
		return handleTFWizard()
	},
}

var tfListCmd = &cobra.Command{
	Use:   "list [tfdir]",
	Short: "List terraform resources",
	Long: `List available aws_instance resources from terraform state.

Examples:
  sshm tf list
  sshm tf list --details
  sshm tf list --fzf`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tfdir := ""
		if len(args) > 0 {
			tfdir = args[0]
		}
		return handleTFList(tfdir, tfListDetails, tfListFZF)
	},
}

var tfAddCmd = &cobra.Command{
	Use:   "add [alias] [key] [tfaddr] [public|private] [user] [port] [tfdir] [tags] [filebase]",
	Short: "Add terraform entry",
	Long: `Add a terraform entry to inventory.

If arguments are missing, starts interactive wizard.
Otherwise adds entry directly.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			// Interactive mode
			return handleTFWizard()
		}
		// Non-interactive mode
		return handleTFAddNonInteractive(args)
	},
}

func init() {
	tfListCmd.Flags().BoolVar(&tfListDetails, "details", false, "Show detailed info (IPs, instance IDs, types)")
	tfListCmd.Flags().BoolVar(&tfListFZF, "fzf", false, "Use fzf for selection")
	
	tfWizardCmd.AddCommand(tfListCmd)
	tfWizardCmd.AddCommand(tfAddCmd)
}

// findTerraformRoot finds terraform root directory by walking up from CWD
func findTerraformRoot(startDir string) (string, error) {
	dir := startDir
	for {
		// Check for terraform indicators
		tfFiles, _ := filepath.Glob(filepath.Join(dir, "*.tf"))
		if len(tfFiles) > 0 {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, ".terraform")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "terraform.tfstate")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		dir = parent
	}
	return "", fmt.Errorf("terraform root not found")
}

// listTerraformResources lists aws_instance resources from terraform state
func listTerraformResources(tfdir string) ([]string, error) {
	if _, err := exec.LookPath("terraform"); err != nil {
		return nil, fmt.Errorf("terraform not found")
	}

	cmd := exec.Command("terraform", "-chdir="+tfdir, "state", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("terraform state list failed: %w", err)
	}

	var resources []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Match aws_instance resources
		if matched, _ := regexp.MatchString(`\.aws_instance\.[^.]*$`, line); matched {
			resources = append(resources, line)
		}
	}

	return resources, nil
}

// getTerraformResourceInfo gets detailed info about a terraform resource
func getTerraformResourceInfo(tfdir, tfaddr string) (publicIP, privateIP, instanceID, instanceType string, err error) {
	cmd := exec.Command("terraform", "-chdir="+tfdir, "state", "show", "-no-color", tfaddr)
	output, err := cmd.Output()
	if err != nil {
		return "", "", "", "", err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if matches := regexp.MustCompile(`^public_ip\s*=\s*"([^"]+)"`).FindStringSubmatch(line); len(matches) == 2 {
			publicIP = matches[1]
		}
		if matches := regexp.MustCompile(`^private_ip\s*=\s*"([^"]+)"`).FindStringSubmatch(line); len(matches) == 2 {
			privateIP = matches[1]
		}
		if matches := regexp.MustCompile(`^id\s*=\s*"([^"]+)"`).FindStringSubmatch(line); len(matches) == 2 {
			instanceID = matches[1]
		}
		if matches := regexp.MustCompile(`^instance_type\s*=\s*"([^"]+)"`).FindStringSubmatch(line); len(matches) == 2 {
			instanceType = matches[1]
		}
	}

	return publicIP, privateIP, instanceID, instanceType, nil
}

// handleTFList handles tf list command
func handleTFList(tfdir string, details bool, useFZF bool) error {
	// Find terraform root if not provided
	if tfdir == "" {
		cwd, _ := os.Getwd()
		var err error
		tfdir, err = findTerraformRoot(cwd)
		if err != nil {
			return fmt.Errorf("could not find terraform root. Specify directory: sshm tf list [tfdir]")
		}
		fmt.Printf("Found terraform directory: %s\n\n", tfdir)
	}

	// Resolve to absolute path
	absTfdir, err := filepath.Abs(tfdir)
	if err != nil {
		return fmt.Errorf("failed to resolve terraform directory: %w", err)
	}
	tfdir = absTfdir

	// List resources
	resources, err := listTerraformResources(tfdir)
	if err != nil {
		return err
	}

	if len(resources) == 0 {
		fmt.Println("No aws_instance resources found")
		return nil
	}

	// Check which resources are already in inventory
	existingAliases := make(map[string]bool)
	allEntries, _ := manager.LoadAll()
	for _, entry := range allEntries {
		if strings.HasPrefix(entry.Target, "tf:") {
			// Extract terraform address
			target := strings.TrimPrefix(entry.Target, "tf:")
			if idx := strings.LastIndex(target, ":"); idx > 0 {
				tfaddr := target[:idx]
				existingAliases[tfaddr] = true
			}
		}
	}

	// Use fzf for selection if requested
	if useFZF {
		if _, err := exec.LookPath("fzf"); err != nil {
			return fmt.Errorf("fzf not found (required for --fzf)")
		}

		// Format for fzf
		var fzfLines []string
		for _, resource := range resources {
			inInv := " "
			if existingAliases[resource] {
				inInv = "✓"
			}

			if details {
				pubIP, privIP, instID, instType, _ := getTerraformResourceInfo(tfdir, resource)
				ip := pubIP
				if ip == "" {
					ip = privIP
				}
				if ip == "" {
					ip = "<pending>"
				}
				line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s", inInv, resource, ip, instID, instType)
				fzfLines = append(fzfLines, line)
			} else {
				line := fmt.Sprintf("%s\t%s", inInv, resource)
				fzfLines = append(fzfLines, line)
			}
		}

		// Run fzf
		fzfCmd := exec.Command("fzf", "--height=60%", "--border", "--header=Select resource (✓ = already in inventory)", "--with-nth=2..")
		fzfCmd.Stderr = os.Stderr
		fzfIn, _ := fzfCmd.StdinPipe()
		
		go func() {
			defer fzfIn.Close()
			for _, line := range fzfLines {
				fmt.Fprintf(fzfIn, "%s\n", line)
			}
		}()

		output, err := fzfCmd.Output()
		if err != nil {
			return nil // User cancelled
		}

		selected := strings.TrimSpace(string(output))
		if selected != "" {
			parts := strings.Split(selected, "\t")
			if len(parts) >= 2 {
				fmt.Println(parts[1]) // Print resource address
			}
		}
		return nil
	}

	// Regular listing
	if details {
		fmt.Println("Legend: ✓ = already in inventory")
		fmt.Println()
		fmt.Printf("%-3s %-50s %-15s %-20s %-15s\n", "INV", "RESOURCE", "PUBLIC_IP", "INSTANCE_ID", "TYPE")
		fmt.Println(strings.Repeat("─", 103))
		for _, resource := range resources {
			inInv := " "
			if existingAliases[resource] {
				inInv = "✓"
			}
			pubIP, privIP, instID, instType, _ := getTerraformResourceInfo(tfdir, resource)
			ip := pubIP
			if ip == "" {
				ip = privIP
			}
			if ip == "" {
				ip = "<pending>"
			}
			fmt.Printf("%-3s %-50s %-15s %-20s %-15s\n", inInv, resource, ip, instID, instType)
		}
	} else {
		fmt.Println("Legend: ✓ = already in inventory")
		fmt.Println()
		fmt.Printf("%-3s %s\n", "INV", "RESOURCE")
		fmt.Println(strings.Repeat("─", 55))
		for _, resource := range resources {
			inInv := " "
			if existingAliases[resource] {
				inInv = "✓"
			}
			fmt.Printf("%-3s %s\n", inInv, resource)
		}
	}

	return nil
}

// handleTFWizard handles interactive terraform add wizard
func handleTFWizard() error {
	fmt.Println("Terraform Add Wizard")
	fmt.Println()

	// Find terraform root
	cwd, _ := os.Getwd()
	tfdir, err := findTerraformRoot(cwd)
	if err != nil {
		fmt.Print("Terraform directory not found. Please specify: ")
		reader := bufio.NewReader(os.Stdin)
		tfdirInput, _ := reader.ReadString('\n')
		tfdir = strings.TrimSpace(tfdirInput)
		if tfdir == "" {
			return fmt.Errorf("terraform directory required")
		}
	} else {
		fmt.Printf("Found terraform directory: %s\n", tfdir)
	}

	absTfdir, err := filepath.Abs(tfdir)
	if err != nil {
		return fmt.Errorf("failed to resolve terraform directory: %w", err)
	}
	tfdir = absTfdir

	// List resources
	resources, err := listTerraformResources(tfdir)
	if err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}

	if len(resources) == 0 {
		fmt.Print("No aws_instance resources found. Enter resource address manually: ")
		reader := bufio.NewReader(os.Stdin)
		tfaddrInput, _ := reader.ReadString('\n')
		tfaddr := strings.TrimSpace(tfaddrInput)
		if tfaddr == "" {
			return fmt.Errorf("resource address required")
		}
		return handleTFWizardContinue(tfdir, tfaddr)
	}

	// Use fzf if available for selection
	var tfaddr string
	if _, err := exec.LookPath("fzf"); err == nil {
		// Use fzf for selection
		var fzfLines []string
		for _, resource := range resources {
			pubIP, privIP, _, _, _ := getTerraformResourceInfo(tfdir, resource)
			ip := pubIP
			if ip == "" {
				ip = privIP
			}
			if ip == "" {
				ip = "<pending>"
			}
			line := fmt.Sprintf("%s\t%s", resource, ip)
			fzfLines = append(fzfLines, line)
		}

		fzfCmd := exec.Command("fzf", "--height=60%", "--border", "--header=Select terraform resource", "--with-nth=1")
		fzfCmd.Stderr = os.Stderr
		fzfIn, _ := fzfCmd.StdinPipe()

		go func() {
			defer fzfIn.Close()
			for _, line := range fzfLines {
				fmt.Fprintf(fzfIn, "%s\n", line)
			}
		}()

		output, err := fzfCmd.Output()
		if err != nil {
			return nil // User cancelled
		}

		selected := strings.TrimSpace(string(output))
		if selected == "" {
			return nil
		}
		parts := strings.Split(selected, "\t")
		tfaddr = parts[0]
	} else {
		// Simple list selection
		fmt.Println("\nAvailable Terraform resources:")
		for i, resource := range resources {
			fmt.Printf("%2d. %s\n", i+1, resource)
		}
		fmt.Print("\nSelect resource number (or enter address manually): ")
		reader := bufio.NewReader(os.Stdin)
		selection, _ := reader.ReadString('\n')
		selection = strings.TrimSpace(selection)

		if num, err := strconv.Atoi(selection); err == nil && num > 0 && num <= len(resources) {
			tfaddr = resources[num-1]
		} else {
			tfaddr = selection
		}
	}

	if tfaddr == "" {
		return fmt.Errorf("resource address required")
	}

	return handleTFWizardContinue(tfdir, tfaddr)
}

// handleTFWizardContinue continues wizard after resource selection
func handleTFWizardContinue(tfdir, tfaddr string) error {
	reader := bufio.NewReader(os.Stdin)

	// Suggest alias from resource name
	suggestedAlias := tfaddr
	if parts := strings.Split(tfaddr, "."); len(parts) > 0 {
		suggestedAlias = parts[len(parts)-1]
	}

	fmt.Printf("\nEnter alias name (suggested: %s): ", suggestedAlias)
	aliasInput, _ := reader.ReadString('\n')
	alias := strings.TrimSpace(aliasInput)
	if alias == "" {
		alias = suggestedAlias
	}

	// Validate alias
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+$`, alias); !matched {
		return fmt.Errorf("invalid alias format (only alphanumeric, dash, underscore, dot allowed)")
	}

	// Check if alias exists
	if _, err := manager.Find(alias); err == nil {
		fmt.Printf("Alias '%s' already exists. Overwrite? [y/N]: ", alias)
		confirm, _ := reader.ReadString('\n')
		if strings.TrimSpace(confirm) != "y" && strings.TrimSpace(confirm) != "Y" {
			return nil
		}
	}

	// Prompt for key
	fmt.Print("\nEnter SSH key path (e.g., ~/.ssh/key.pem): ")
	keyInput, _ := reader.ReadString('\n')
	key := strings.TrimSpace(keyInput)
	if key == "" {
		return fmt.Errorf("key path required")
	}

	// Expand ~
	if strings.HasPrefix(key, "~") {
		home, _ := os.UserHomeDir()
		key = strings.Replace(key, "~", home, 1)
	}

	// Resolve to absolute path
	absKey, err := filepath.Abs(key)
	if err != nil {
		return fmt.Errorf("failed to resolve key path: %w", err)
	}
	key = absKey

	// Validate key exists
	if _, err := os.Stat(key); os.IsNotExist(err) {
		return fmt.Errorf("key file not found: %s", key)
	}

	// Prompt for IP mode
	fmt.Print("\nIP mode [public/private] (default: public): ")
	modeInput, _ := reader.ReadString('\n')
	mode := strings.TrimSpace(modeInput)
	if mode == "" {
		mode = "public"
	}
	if mode != "public" && mode != "private" {
		mode = "public"
	}

	// Prompt for user
	fmt.Print("\nSSH user (common: ubuntu, ec2-user, admin, root) (optional, press Enter to skip): ")
	userInput, _ := reader.ReadString('\n')
	user := strings.TrimSpace(userInput)

	// Prompt for port
	fmt.Print("\nSSH port (default: 22) (optional, press Enter to skip): ")
	portInput, _ := reader.ReadString('\n')
	port := strings.TrimSpace(portInput)

	// Prompt for tags
	fmt.Print("\nTags (comma-separated, e.g., prod,web,terraform) (optional, press Enter to skip): ")
	tagsInput, _ := reader.ReadString('\n')
	tags := strings.TrimSpace(tagsInput)

	// Prompt for filebase
	fmt.Print("\nInventory filebase (default: terraform): ")
	filebaseInput, _ := reader.ReadString('\n')
	filebase := strings.TrimSpace(filebaseInput)
	if filebase == "" {
		filebase = "terraform"
	}

	// Show summary
	fmt.Println("\nSummary:")
	fmt.Printf("  Alias    : %s\n", alias)
	fmt.Printf("  Key      : %s\n", key)
	fmt.Printf("  Resource : %s\n", tfaddr)
	fmt.Printf("  Mode     : %s\n", mode)
	if user == "" {
		fmt.Printf("  User     : -\n")
	} else {
		fmt.Printf("  User     : %s\n", user)
	}
	if port == "" {
		fmt.Printf("  Port     : -\n")
	} else {
		fmt.Printf("  Port     : %s\n", port)
	}
	if tags == "" {
		fmt.Printf("  Tags     : -\n")
	} else {
		fmt.Printf("  Tags     : %s\n", tags)
	}
	fmt.Printf("  Filebase : %s\n", filebase)
	fmt.Printf("  TF Dir   : %s\n", tfdir)
	fmt.Print("\nAdd this entry? [y/N]: ")
	confirm, _ := reader.ReadString('\n')
	if strings.TrimSpace(confirm) != "y" && strings.TrimSpace(confirm) != "Y" {
		fmt.Println("Cancelled")
		return nil
	}

	// Validate terraform state
	cmd := exec.Command("terraform", "-chdir="+tfdir, "state", "show", tfaddr)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: terraform state show failed for: %s\n", tfaddr)
		fmt.Print("Continue anyway? [y/N]: ")
		confirm, _ := reader.ReadString('\n')
		if strings.TrimSpace(confirm) != "y" && strings.TrimSpace(confirm) != "Y" {
			return nil
		}
	}

	// Create entry
	entry := inventory.NewEntry()
	entry.Alias = alias
	entry.Type = "tf"
	entry.Target = fmt.Sprintf("%s:%s", tfaddr, mode)
	entry.Key = key
	entry.User = user
	entry.Port = port
	entry.Workdir = tfdir
	entry.Tags = tags

	// Add entry
	if err := manager.Add(entry, filebase); err != nil {
		return fmt.Errorf("failed to add entry: %w", err)
	}

	fmt.Printf("\n✓ Added TF entry: %s → tf:%s:%s (dir: %s)\n", alias, tfaddr, mode, tfdir)
	return nil
}

// handleTFAddNonInteractive handles non-interactive tf add
func handleTFAddNonInteractive(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("non-interactive mode requires: alias key tfaddr [public|private] [user] [port] [tfdir] [tags] [filebase]")
	}

	alias := args[0]
	key := args[1]
	tfaddr := args[2]
	mode := "public"
	if len(args) > 3 {
		mode = args[3]
	}
	if mode != "public" && mode != "private" {
		mode = "public"
	}

	user := ""
	if len(args) > 4 {
		user = args[4]
	}
	port := ""
	if len(args) > 5 {
		port = args[5]
	}
	tfdir := ""
	if len(args) > 6 {
		tfdir = args[6]
	}
	tags := ""
	if len(args) > 7 {
		tags = args[7]
	}
	filebase := "terraform"
	if len(args) > 8 {
		filebase = args[8]
	}

	// Find terraform root if not provided
	if tfdir == "" {
		cwd, _ := os.Getwd()
		var err error
		tfdir, err = findTerraformRoot(cwd)
		if err != nil {
			return fmt.Errorf("could not find terraform root (pass tfdir explicitly)")
		}
	}

	absTfdir, err := filepath.Abs(tfdir)
	if err != nil {
		return fmt.Errorf("failed to resolve terraform directory: %w", err)
	}
	tfdir = absTfdir

	// Expand and resolve key
	if strings.HasPrefix(key, "~") {
		home, _ := os.UserHomeDir()
		key = strings.Replace(key, "~", home, 1)
	}
	absKey, err := filepath.Abs(key)
	if err != nil {
		return fmt.Errorf("failed to resolve key path: %w", err)
	}
	key = absKey

	// Validate key exists
	if _, err := os.Stat(key); os.IsNotExist(err) {
		return fmt.Errorf("key file not found: %s", key)
	}

	// Create entry
	entry := inventory.NewEntry()
	entry.Alias = alias
	entry.Type = "tf"
	entry.Target = fmt.Sprintf("%s:%s", tfaddr, mode)
	entry.Key = key
	entry.User = user
	entry.Port = port
	entry.Workdir = tfdir
	entry.Tags = tags

	// Add entry
	if err := manager.Add(entry, filebase); err != nil {
		return fmt.Errorf("failed to add entry: %w", err)
	}

	fmt.Printf("Added TF: %s → tf:%s:%s (dir: %s)\n", alias, tfaddr, mode, tfdir)
	return nil
}

