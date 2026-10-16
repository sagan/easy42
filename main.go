package main

import (
	"easy42/cmd"
	appVersion "easy42/internal/version"
)

// Injected by ldflags if targeting main package
var (
	version = ""
	commit  = ""
	date    = ""
	builtBy = ""
)

func init() {
	if version != "" {
		appVersion.Version = version
	}
	if commit != "" {
		appVersion.Commit = commit
	}
	if date != "" {
		appVersion.Date = date
	}
	if builtBy != "" {
		appVersion.BuiltBy = builtBy
	}
}

func main() {
	cmd.Execute()
}
