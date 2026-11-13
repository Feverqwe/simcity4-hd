package sc4

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"time"
)

const setupReleaseAPI = "https://api.github.com/repos/Feverqwe/simcity4-hd/releases/latest"

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

func checkSetupUpdate(version string, out io.Writer) error {
	client := &http.Client{Timeout: 6 * time.Second}
	return reportSetupUpdate(version, setupReleaseAPI, client, out)
}

func reportSetupUpdate(version, endpoint string, client *http.Client, out io.Writer) error {
	current, prerelease, ok := setupReleaseVersion(version)
	if !ok {
		return nil
	}

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
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return err
	}
	latest, latestPrerelease, ok := setupReleaseVersion(release.Tag)
	if !ok || latestPrerelease || release.Prerelease || release.Draft {
		return nil
	}
	if comparison := slices.Compare(latest, current); comparison > 0 || (comparison == 0 && prerelease) {
		_, err := fmt.Fprintln(out, release.Tag)
		return err
	}
	return nil
}
