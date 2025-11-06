package utils

import (
	"fmt"

	"github.com/syndtr/gocapability/capability"
)

func CheckCapabilities(capabilities ...capability.Cap) (bool, error) {
	if len(capabilities) == 0 {
		return true, nil
	}

	caps, err := capability.NewPid2(0) // 0 == current process
	if err != nil {
		return false, err
	}
	if err = caps.Load(); err != nil {
		return false, err
	}

	var missing []string
	for _, c := range capabilities {
		if !caps.Get(capability.EFFECTIVE, c) {
			missing = append(missing, c.String())
		}
	}

	if len(missing) > 0 {
		return false, fmt.Errorf("required capabilities are missing: %v", missing)
	}
	return true, nil
}
