// The Go-built tools the shared recipes run, pinned as tool directives
// in a module of their own so their dependency graph never reaches the
// project's go.mod (book/tooling.md).
module github.com/mycophonic/saprobe-alac/tools

go 1.26.8

tool (
	github.com/forkcloser/dot/cmd/dot
	github.com/forkcloser/godolint/cmd/godolint
	github.com/google/go-licenses/v2
	github.com/vbatts/git-validation
	golang.org/x/tools/cmd/deadcode
	golang.org/x/vuln/cmd/govulncheck
)

require (
	github.com/alecthomas/kong v1.16.1 // indirect
	github.com/containerd/typeurl/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/forkcloser/dot v1.3.0 // indirect
	github.com/forkcloser/go-graphviz v0.5.1 // indirect
	github.com/forkcloser/godolint v0.4.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/go-licenses/v2 v2.0.1 // indirect
	github.com/google/licenseclassifier/v2 v2.0.0 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lmittmann/tint v1.2.1 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/moby/buildkit v0.34.0 // indirect
	github.com/mycophonic/primordium v0.14.0 // indirect
	github.com/otiai10/copy v1.10.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/sergi/go-diff v1.2.0 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
	github.com/vbatts/git-validation v1.2.2 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/telemetry v0.0.0-20261010193011-f71dc92da45e // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	golang.org/x/vuln v1.9.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	mvdan.cc/sh/v3 v3.14.1 // indirect
)
