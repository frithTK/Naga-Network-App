module naga.network

go 1.26.0

require (
	github.com/amnezia-vpn/amneziawg-go/v3 v3.1.20260828
	github.com/refraction-networking/utls v1.8.2
	golang.org/x/net v0.58.0
	golang.org/x/sys v0.48.0
	golang.zx2c4.com/wireguard v0.0.0-20260522210424-ecfc5a8d5446
)

require (
	github.com/andybalholm/brotli v1.0.6 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
	gvisor.dev/gvisor v0.0.0-20250606233247-e3c4c4cad86f // indirect
)

// GO-2026-5932: golang.org/x/crypto/openpgp is unmaintained. It is not in the
// Naga call path; do not vendor a fork for this advisory.

// gVisor master pseudo-versions include mixed-package test files that break
// Go 1.27 imports. Use the same importable snapshot as wireguard-go.
replace gvisor.dev/gvisor => gvisor.dev/gvisor v0.0.0-20250503011706-39ed1f5ac29c
