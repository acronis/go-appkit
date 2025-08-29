/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package libinfo

import (
	"debug/buildinfo"
	"regexp"
	"sync"

	"runtime/debug"

	"github.com/prometheus/client_golang/prometheus"
)

const libShortName = "go-appkit"

const moduleName = "github.com/acronis/" + libShortName

const PrometheusLibVersionLabel = "go_appkit_version"

func AddPrometheusLibVersionLabel(labels prometheus.Labels) prometheus.Labels {
	labelsCopy := make(prometheus.Labels, len(labels))
	for k, v := range labels {
		labelsCopy[k] = v
	}
	labelsCopy[PrometheusLibVersionLabel] = GetLibVersion()
	return labelsCopy
}

var libVersion string
var libVersionOnce sync.Once

func GetLibVersion() string {
	libVersionOnce.Do(initLibVersion)
	return libVersion
}

func initLibVersion() {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		libVersion = extractLibVersion(buildInfo, moduleName)
	}
	if libVersion == "" {
		libVersion = "v0.0.0"
	}
}

// extractLibVersion extracts the version of the given module from the build info.
// It expects the module name to be in the form "moduleName" or "moduleName/vX" where X is a major version number.
// This format is used by Go modules to indicate major version changes.
func extractLibVersion(buildInfo *buildinfo.BuildInfo, modName string) string {
	if buildInfo == nil {
		return ""
	}
	re, err := regexp.Compile(`^` + regexp.QuoteMeta(modName) + `(/v[0-9]+)?$`)
	if err != nil {
		return "" // should never happen
	}
	for _, dep := range buildInfo.Deps {
		if re.MatchString(dep.Path) {
			return dep.Version
		}
	}
	return ""
}
