module github.com/minio/minio-go/examples/s3

go 1.22

toolchain go1.22.7

// Overridden by `replace` below, to point all versions at the local minio-go source, so version shouldn't matter here.
require github.com/minio/minio-go/v7 v7.0.73

require (
	github.com/cheggaaa/pb v1.0.29
	github.com/minio/sio v0.3.0
	golang.org/x/crypto v0.33.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ini/ini v1.67.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/rs/xid v1.6.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/minio/minio-go/v7 => ../..
