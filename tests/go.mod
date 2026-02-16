module github.com/mycophonic/saprobe-alac/tests

go 1.25.7

replace github.com/mycophonic/saprobe-alac => ../

require (
	github.com/mycophonic/agar v0.1.4
	github.com/mycophonic/saprobe-alac v0.0.0-00010101000000-000000000000
)

require (
	github.com/containerd/nerdctl/mod/tigron v0.0.0-20260121031139-a630881afd01 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/mycophonic/primordium v0.0.0-20260215224452-2f1f734cee4e // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/term v0.39.0 // indirect
	golang.org/x/text v0.33.0 // indirect
)
