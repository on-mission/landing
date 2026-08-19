package adapters

import (
	"os"
	"strings"

	"github.com/on-mission/landing/internal/harness"
)

func absentDetection() harness.Detection {
	return harness.Detection{Status: harness.DetectionAbsent, Capacity: harness.UnknownCapacity()}
}

func capacityDetection(path string, capacity harness.Capacity, detail string, authenticationFailure func(string, string) bool) harness.Detection {
	if capacity.IsKnown() {
		return harness.Detection{
			Status:   harness.DetectionReady,
			Path:     path,
			Capacity: capacity,
		}
	}
	if authenticationFailure(detail, "") {
		return harness.Detection{
			Status:   harness.DetectionUnauthenticated,
			Path:     path,
			Capacity: harness.UnknownCapacity(),
			Detail:   detail,
		}
	}

	return harness.Detection{
		Status:   harness.DetectionUnreadable,
		Path:     path,
		Capacity: harness.UnknownCapacity(),
		Detail:   detail,
	}
}

func executableIn(path, name string) string {
	for _, directory := range strings.Split(path, string(os.PathListSeparator)) {
		if directory == "" {
			continue
		}
		candidate, err := executablePath(directory, name)
		if err != nil || !isExecutable(candidate) {
			continue
		}

		return candidate
	}

	return ""
}

func spawnPath(prefix []string) string {
	directories := uniqueDirectories(append(append([]string(nil), prefix...), pathDirectories()...))
	for _, directory := range directories {
		info, err := os.Stat(directory)
		if err == nil && info.IsDir() {
			return strings.Join(directories, string(os.PathListSeparator))
		}
	}

	workingDirectory, err := os.Getwd()
	if err == nil {
		return strings.Join(append(directories, workingDirectory), string(os.PathListSeparator))
	}

	return strings.Join(append(directories, os.TempDir()), string(os.PathListSeparator))
}
