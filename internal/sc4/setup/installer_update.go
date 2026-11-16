package setup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"time"
)

const setupReleasesAPI = "https://api.github.com/repos/Feverqwe/simcity4-hd/releases"

var setupVersionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func setupReleaseVersion(tag string) ([]uint64, bool, bool) {
	match := setupVersionPattern.FindStringSubmatch(tag)
	if match == nil {
		return nil, false, false
	}
	version := make([]uint64, 3)
	for i, value := range match[1:4] {
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return nil, false, false
		}
		version[i] = n
	}
	return version, match[4] != "", true
}

func CheckUpdate(version string, out io.Writer) error {
	client := &http.Client{Timeout: 6 * time.Second}
	if runtime.GOOS == "darwin" {
		arch := runtime.GOARCH
		if arch == "amd64" {
			arch = "x86_64"
		}
		asset := "SimCity4UHD-Setup-macos-" + arch + ".zip"
		return reportMacSetupUpdate(version, setupReleasesAPI+"?per_page=100", asset, client, out)
	}
	return reportSetupUpdate(version, setupReleasesAPI+"/latest", client, out)
}

type setupRelease struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"assets"`
}

func (r setupRelease) stableVersion() []uint64 {
	version, prerelease, ok := setupReleaseVersion(r.Tag)
	if !ok || prerelease || r.Draft || r.Prerelease {
		return nil
	}
	return version
}

func reportSetupUpdate(version, endpoint string, client *http.Client, out io.Writer) error {
	current, prerelease, ok := setupReleaseVersion(version)
	if !ok {
		return nil
	}
	var release setupRelease
	if err := fetchSetupReleases(client, endpoint, &release); err != nil {
		return err
	}
	latest := release.stableVersion()
	if latest != nil && newerSetupRelease(latest, current, prerelease) {
		_, err := fmt.Fprintln(out, release.Tag)
		return err
	}
	return nil
}

func reportMacSetupUpdate(version, endpoint, asset string, client *http.Client, out io.Writer) error {
	current, prerelease, ok := setupReleaseVersion(version)
	if !ok {
		return nil
	}
	var releases []setupRelease
	if err := fetchSetupReleases(client, endpoint, &releases); err != nil {
		return err
	}
	var latest []uint64
	var tag string
	for _, release := range releases {
		candidate := release.stableVersion()
		if candidate == nil || !newerSetupRelease(candidate, current, prerelease) || slices.Compare(candidate, latest) <= 0 {
			continue
		}
		for _, file := range release.Assets {
			if file.Name == asset && file.State == "uploaded" {
				latest, tag = candidate, release.Tag
				break
			}
		}
	}
	if tag != "" {
		_, err := fmt.Fprintln(out, tag)
		return err
	}
	return nil
}

func newerSetupRelease(candidate, current []uint64, prerelease bool) bool {
	comparison := slices.Compare(candidate, current)
	return comparison > 0 || (comparison == 0 && prerelease)
}

func fetchSetupReleases(client *http.Client, endpoint string, target any) error {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SimCity4UHD-Setup")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update check: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(target)
}
