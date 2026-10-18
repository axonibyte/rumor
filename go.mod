module forge.axonibyte.com/axonibyte/rumor

// RAISED 1.25 -> 1.26.0 in G3, and only because both conditions the earlier
// comment set are now met: forge-01_jail is measured at go1.26.8 (rook
// go-port/2), and OPA v1.21.2 declares `go 1.26.0`, so the dependency sets the
// floor rather than a preference of mine. With GOTOOLCHAIN=local a box below
// 1.26 now REFUSES rather than downloading a toolchain, which is the intended
// behaviour: the build fails loudly on an under-provisioned runner.
go 1.26.0

require github.com/open-policy-agent/opa v1.21.2

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/gobwas/glob v1.0.0 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.4 // indirect
	github.com/lestrrat-go/dsig v1.4.0 // indirect
	github.com/lestrrat-go/dsig-secp256k1 v1.0.0 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc/v3 v3.0.6 // indirect
	github.com/lestrrat-go/jwx/v3 v3.3.0 // indirect
	github.com/lestrrat-go/option/v2 v2.0.0 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/tchap/go-patricia/v2 v2.3.3 // indirect
	github.com/valyala/fastjson v1.6.10 // indirect
	github.com/vektah/gqlparser/v2 v2.5.37 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/yashtewari/glob-intersection v0.2.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
