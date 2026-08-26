package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var githubReleasesURL = "https://api.github.com/repos/CloakHQ/cloakbrowser/releases?per_page=30"

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func LatestStableVersion(client *http.Client) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequest(http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "opencode-go-manager")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("查询 CloakBrowser 版本失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("查询 CloakBrowser 版本失败: GitHub HTTP %d", resp.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", fmt.Errorf("解析版本列表失败: %w", err)
	}
	latest, ok := pickLatestProVersion(releases)
	if !ok {
		return "", fmt.Errorf("GitHub 发布列表里没有可用的 Pro 版本")
	}
	return latest, nil
}

func pickLatestProVersion(releases []githubRelease) (string, bool) {
	latest := ""
	for _, rel := range releases {
		if rel.Draft || rel.Prerelease {
			continue
		}
		ver, pro, ok := parseReleaseTag(rel.TagName)
		if !ok || !pro {
			continue
		}
		if latest == "" || CompareVersion(ver, latest) > 0 {
			latest = ver
		}
	}
	return latest, latest != ""
}

func parseReleaseTag(tag string) (version string, pro bool, ok bool) {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "chromium-v")
	tag = strings.TrimPrefix(tag, "v")
	pro = strings.HasSuffix(tag, "-pro")
	tag = strings.TrimSuffix(tag, "-pro")
	if !looksLikeCloakVersion(tag) {
		return "", false, false
	}
	return tag, pro, true
}

func looksLikeCloakVersion(v string) bool {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) < 4 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

func CompareVersion(a, b string) int {
	as := versionParts(a)
	bs := versionParts(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	raw := strings.Split(strings.TrimSpace(v), ".")
	out := make([]int, 0, len(raw))
	for _, p := range raw {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}
