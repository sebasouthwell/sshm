package cli

import (
	"fmt"

	"github.com/Sebasouthwell/sshm/internal/provider"
)

var (
	providers map[string]provider.Provider
)

func init() {
	providers = make(map[string]provider.Provider)
	
	// Register providers
	providers["ssh"] = provider.NewSSHProvider()
	providers["tf"] = provider.NewTFProvider()
	providers["ssm"] = provider.NewSSMProvider()
	providers["docker"] = provider.NewDockerProvider()
	providers["kube"] = provider.NewKubeProvider()
}

// GetProvider returns a provider by name
func GetProvider(name string) (provider.Provider, error) {
	p, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return p, nil
}
