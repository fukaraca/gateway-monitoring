package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/syndtr/gocapability/capability"
)

func TestCheckCapabilities_NoArgs(t *testing.T) {
	ok, err := CheckCapabilities()
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCheckCapabilities_SingleCap(t *testing.T) {
	c := capability.CAP_NET_ADMIN

	ok, err := CheckCapabilities(c)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCheckCapabilities_SysAdminCap(t *testing.T) {
	c := capability.CAP_SYS_ADMIN

	ok, err := CheckCapabilities(c)
	require.Error(t, err)
	require.False(t, ok)
}

func TestCheckCapabilities_MultipleCaps(t *testing.T) {
	caps := []capability.Cap{
		capability.CAP_NET_ADMIN,
		capability.CAP_NET_RAW,
	}

	ok, err := CheckCapabilities(caps...)
	require.NoError(t, err)
	require.True(t, ok)
}
