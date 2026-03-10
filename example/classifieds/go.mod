module github.com/romshark/datapages/example/classifieds

go 1.26.1

replace github.com/romshark/datapages => ../../

// Required by the demo application
require github.com/oklog/ulid/v2 v2.1.1

// Required by Datapages
require (
	github.com/a-h/templ v0.3.1001
	github.com/nats-io/nats.go v1.49.0
	github.com/prometheus/client_golang v1.23.2
	github.com/starfederation/datastar-go v1.1.0
	golang.org/x/crypto v0.48.0
	golang.org/x/sync v0.20.0
)

require github.com/romshark/datapages v0.4.1

require (
	github.com/CAFxX/httpcompression v0.0.9 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	golang.org/x/sys v0.42.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
