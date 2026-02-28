package main

import "runtime"

var (
	Version   = "dev"
	GitSHA    = "none"
	BuildTime = "unknown"
)

type VersionInfo struct {
	Version   string `json:"version"`
	GitSHA    string `json:"gitSHA"`
	BuildTime string `json:"buildTime"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
}

func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version:   Version,
		GitSHA:    GitSHA,
		BuildTime: BuildTime,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
}
