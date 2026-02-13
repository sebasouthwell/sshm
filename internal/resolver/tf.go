package resolver

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	tfCacheTTL = 300 // seconds, override via SSHM_TF_CACHE_TTL
)

func init() {
	if ttl := os.Getenv("SSHM_TF_CACHE_TTL"); ttl != "" {
		if val, err := strconv.Atoi(ttl); err == nil {
			tfCacheTTL = val
		}
	}
}

// ResolveTerraformIP resolves a Terraform resource address to an IP
func ResolveTerraformIP(workdir, tfaddr, mode string) (string, error) {
	// Validate mode
	if mode != "public" && mode != "private" {
		mode = "public"
	}

	// Check cache
	cacheKey := fmt.Sprintf("%s:%s:%s", workdir, tfaddr, mode)
	if ip := getCachedIP(cacheKey); ip != "" {
		return ip, nil
	}

	// Resolve via terraform
	ip, err := resolveFromTerraform(workdir, tfaddr, mode)
	if err != nil {
		return "", err
	}

	// Cache the result
	setCachedIP(cacheKey, ip)

	return ip, nil
}

// resolveFromTerraform calls terraform state show and extracts IP
func resolveFromTerraform(workdir, tfaddr, mode string) (string, error) {
	// Check if terraform is available
	if _, err := exec.LookPath("terraform"); err != nil {
		return "", fmt.Errorf("terraform not found (needed to resolve tf: hosts); install via: brew install terraform")
	}

	// Run terraform state show
	cmd := exec.Command("terraform", "-chdir="+workdir, "state", "show", "-no-color", tfaddr)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("terraform state show failed for %s: %w", tfaddr, err)
	}

	// Parse output for IPs
	pubIP, privIP := parseTerraformOutput(string(output))
	if pubIP == "" && privIP == "" {
		return "", fmt.Errorf("no public/private IP found in state for %s", tfaddr)
	}

	// Select IP based on mode
	var chosen string
	if mode == "private" {
		chosen = privIP
		if chosen == "" {
			chosen = pubIP
		}
	} else {
		chosen = pubIP
		if chosen == "" {
			chosen = privIP
		}
	}

	if chosen == "" {
		return "", fmt.Errorf("no IP found for mode %s in state for %s", mode, tfaddr)
	}

	return chosen, nil
}

// parseTerraformOutput parses terraform state show output for IPs
func parseTerraformOutput(output string) (publicIP, privateIP string) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Match: public_ip = "1.2.3.4"
		if matches := regexp.MustCompile(`^public_ip\s*=\s*"([^"]+)"`).FindStringSubmatch(line); len(matches) == 2 {
			publicIP = matches[1]
		}

		// Match: private_ip = "10.0.0.1"
		if matches := regexp.MustCompile(`^private_ip\s*=\s*"([^"]+)"`).FindStringSubmatch(line); len(matches) == 2 {
			privateIP = matches[1]
		}
	}

	return publicIP, privateIP
}

// getCachedIP retrieves cached IP if still valid
func getCachedIP(cacheKey string) string {
	cacheFile := getCacheFile(cacheKey)

	info, err := os.Stat(cacheFile)
	if err != nil {
		return ""
	}

	// Check if cache is still valid
	age := time.Since(info.ModTime()).Seconds()
	if age > float64(tfCacheTTL) {
		os.Remove(cacheFile) // Clean up expired cache
		return ""
	}

	// Read cached IP
	data, err := ioutil.ReadFile(cacheFile)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// setCachedIP caches an IP
func setCachedIP(cacheKey, ip string) {
	cacheFile := getCacheFile(cacheKey)

	// Ensure cache directory exists
	cacheDir := filepath.Dir(cacheFile)
	os.MkdirAll(cacheDir, 0755)

	// Write cache file
	ioutil.WriteFile(cacheFile, []byte(ip), 0644)
}

// getCacheFile returns the cache file path for a cache key
func getCacheFile(cacheKey string) string {
	tmpDir := os.TempDir()
	if tmpDir == "" {
		tmpDir = "/tmp"
	}

	// Create hash of cache key
	hash := md5.Sum([]byte(cacheKey))
	hashStr := fmt.Sprintf("%x", hash)

	return filepath.Join(tmpDir, fmt.Sprintf("sshm-tf-%s", hashStr))
}
