package resolver

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// ResolveSSMInstance resolves a tag selector to instance IDs
// Returns instance IDs sorted by instance-id ascending
func ResolveSSMInstance(selector string, profile, region string) ([]string, error) {
	// Parse selector: tag:Key=Value
	if !strings.HasPrefix(selector, "tag:") {
		return nil, fmt.Errorf("invalid SSM selector format: %s (expected tag:Key=Value)", selector)
	}

	tagPart := strings.TrimPrefix(selector, "tag:")
	parts := strings.SplitN(tagPart, "=", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid tag selector format: %s (expected Key=Value)", tagPart)
	}

	tagKey := parts[0]
	tagValue := parts[1]

	// Build AWS CLI command
	args := []string{"ec2", "describe-instances"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}
	args = append(args, "--filters", fmt.Sprintf("Name=tag:%s,Values=%s", tagKey, tagValue))
	args = append(args, "--query", "Reservations[*].Instances[*].[InstanceId,Tags[?Key=='Name'].Value|[0]]")
	args = append(args, "--output", "json")

	cmd := exec.Command("aws", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("aws ec2 describe-instances failed: %w", err)
	}

	// Parse JSON output
	var results [][]interface{}
	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("failed to parse AWS CLI output: %w", err)
	}

	var instanceIDs []string
	for _, reservation := range results {
		for _, instance := range reservation {
			if instanceArray, ok := instance.([]interface{}); ok && len(instanceArray) > 0 {
				if instanceID, ok := instanceArray[0].(string); ok {
					instanceIDs = append(instanceIDs, instanceID)
				}
			}
		}
	}

	if len(instanceIDs) == 0 {
		return nil, fmt.Errorf("no instances found matching tag selector: %s", selector)
	}

	// Sort by instance-id ascending
	sort.Strings(instanceIDs)

	return instanceIDs, nil
}

// ResolveSSMInstanceFirst resolves selector and returns first match
func ResolveSSMInstanceFirst(selector string, profile, region string) (string, error) {
	instances, err := ResolveSSMInstance(selector, profile, region)
	if err != nil {
		return "", err
	}
	return instances[0], nil
}
