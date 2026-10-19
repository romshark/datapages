package inmem_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/modules/assetcompress"
	"github.com/romshark/datapages/modules/assetcompress/inmem"
	"github.com/romshark/datapages/modules/assetcompress/internal/storetest"
)

// TestStore covers the [assetcompress.Store] contract.
func TestStore(t *testing.T) {
	storetest.Run(t, func(*testing.T) assetcompress.Store { return inmem.New() })
}

// TestBytes tests that Bytes returns a stored copy and reports a missing one.
func TestBytes(t *testing.T) {
	t.Parallel()

	s := inmem.New()
	require.NoError(t, s.Put("a.br6", strings.NewReader("copy")))
	b, err := s.Bytes("a.br6")
	require.NoError(t, err)
	require.Equal(t, "copy", string(b))
	_, err = s.Bytes("b.br6")
	require.ErrorIs(t, err, fs.ErrNotExist)
}
