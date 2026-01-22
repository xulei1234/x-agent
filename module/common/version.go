package common

import "fmt"

var (
	Version   = "0.1.2"
	GitCommit string
	GoVersion string
	BuildTime string
	BuildHost string
)

func VersionInfo() string {
	return fmt.Sprintf("Verion: %s\nGitCommit: %s\nGoVersion: %s\nBuildTime: %s\nBuildHost:%s\n",
		Version,
		GitCommit,
		GoVersion,
		BuildTime,
		BuildHost,
	)

}
