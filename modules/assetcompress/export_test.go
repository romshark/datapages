package assetcompress

import "time"

// SetWriteRetry sets the first pause between write attempts.
// Call it before the first request.
func SetWriteRetry(c *Compressor, d time.Duration) { c.writeRetry = d }

// WaitBackground waits for the goroutines that requests started.
func WaitBackground(c *Compressor) { c.background.Wait() }
