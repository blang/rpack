package cmd

// BuildVersion is injected at compile time via ldflags. Defaults to "dev"
// so builds without ldflags (e.g. `go install .`) report a non-empty version.
var BuildVersion = "dev"

// BuildCommit is injected at compile time via ldflags.
var BuildCommit string

// BuildTime is injected at compile time via ldflags.
var BuildTime string
