module github.com/romshark/datapages/internal/acceptance/hreflocals

go 1.27.0

replace github.com/romshark/datapages => ../../../

require (
	// Required by Datapages
	github.com/a-h/templ v0.3.1020
	github.com/nats-io/nats.go v1.53.1
	github.com/romshark/datapages v0.9.4
	github.com/starfederation/datastar-go v1.2.2
	github.com/stretchr/testify v1.12.1
	golang.org/x/sync v0.22.0
)

require (
	github.com/CAFxX/httpcompression v0.0.9 // indirect
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/nats-io/nkeys v0.4.16 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
