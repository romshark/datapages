package inmem_test

import (
	"testing"

	"github.com/romshark/datapages/modules/assetcompress"
	"github.com/romshark/datapages/modules/assetcompress/inmem"
	"github.com/romshark/datapages/modules/assetcompress/internal/storetest"
)

// TestStore tests the [assetcompress.Store] contract.
func TestStore(t *testing.T) {
	storetest.Run(t, func(*testing.T) assetcompress.Store { return inmem.New() })
}
