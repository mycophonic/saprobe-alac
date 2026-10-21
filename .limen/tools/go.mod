// The Go-built tools the shared recipes run, pinned as tool directives
// in a module of their own so their dependency graph never reaches the
// project's go.mod (book/tooling.md).
module github.com/mycophonic/saprobe-alac/tools

go 1.26.8

tool (
	github.com/forkcloser/dot/cmd/dot
	github.com/forkcloser/git-validation/cmd/git-validation
	github.com/forkcloser/go-licenses/cmd/go-licenses
	github.com/forkcloser/godolint/cmd/godolint
	golang.org/x/tools/cmd/deadcode
	golang.org/x/vuln/cmd/govulncheck
)

require (
	github.com/alecthomas/kong v1.16.1 // indirect
	github.com/containerd/typeurl/v2 v2.3.0 // indirect
	github.com/forkcloser/dot v1.3.1 // indirect
	github.com/forkcloser/git-validation v0.1.1 // indirect
	github.com/forkcloser/go-graphviz v0.5.2 // indirect
	github.com/forkcloser/go-licenses v0.1.0 // indirect
	github.com/forkcloser/godolint v0.5.1 // indirect
	github.com/lmittmann/tint v1.2.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/moby/buildkit v0.34.0 // indirect
	github.com/mycophonic/primordium v0.15.1 // indirect
	github.com/otiai10/copy v1.14.1 // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/telemetry v0.0.0-20261010193011-f71dc92da45e // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	golang.org/x/vuln v1.9.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	mvdan.cc/sh/v3 v3.14.1 // indirect
)
