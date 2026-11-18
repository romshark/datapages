module github.com/romshark/datapages/example/offline-cache

go 1.27.4

replace github.com/romshark/datapages => ../../

// Required by Datapages
require (
	github.com/a-h/templ v0.3.1118
	github.com/romshark/datapages v0.11.3
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/stretchr/testify v1.12.1
	golang.org/x/crypto v0.59.0
)

require (
	github.com/andybalholm/brotli v1.3.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/klauspost/compress v1.20.4 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pandatix/go-abnf v0.5.5 // indirect
	github.com/prometheus/client_golang v1.25.0 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.74.0 // indirect
	github.com/prometheus/procfs v0.23.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
