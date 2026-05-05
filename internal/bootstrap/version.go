package bootstrap

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type VersionInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

func NewVersionInfo() VersionInfo {
	return VersionInfo{Version: Version, Commit: Commit, BuildDate: BuildDate}
}
