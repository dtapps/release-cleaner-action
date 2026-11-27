package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	releasesPageSize = 100
	githubAPIVersion = "2022-11-28"
)

type Release struct {
	ID        int64  `json:"id"`
	TagName   string `json:"tag_name"`
	CreatedAt string `json:"created_at"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	apiEndpoint := strings.TrimRight(getEnv("GITHUB_API_URL", "https://api.github.com"), "/")
	token := getEnv("INPUT_TOKEN", os.Getenv("GITHUB_TOKEN"))
	slug := os.Getenv("GITHUB_REPOSITORY")

	mode := getEnv("INPUT_MODE", "by_count")
	keep, keepErr := strconv.Atoi(getEnv("INPUT_KEEP", "5"))
	days, daysErr := strconv.Atoi(getEnv("INPUT_DAYS", "30"))
	regexPattern := os.Getenv("INPUT_REGEX")
	protectPattern := getEnv("INPUT_PROTECT", `^(v\d+\.\d+\.\d+|latest|stable)$`)
	deleteTag, deleteTagErr := parseBoolEnv("INPUT_DELETE_GIT_TAG", false)
	dryRun, dryRunErr := parseBoolEnv("INPUT_DRY_RUN", true)

	var errs []string
	if token == "" {
		errs = append(errs, "缺少必要参数 token（GitHub Actions 中通过 with.token 传入，默认为 github.token）")
	}
	if slug == "" {
		errs = append(errs, "缺少必要环境变量 GITHUB_REPOSITORY（GitHub Actions 自动注入）")
	}
	if keepErr != nil || keep < 0 {
		errs = append(errs, fmt.Sprintf("参数 keep 必须是非负整数，当前值: %q", os.Getenv("INPUT_KEEP")))
	}
	if daysErr != nil || days < 0 {
		errs = append(errs, fmt.Sprintf("参数 days 必须是非负整数，当前值: %q", os.Getenv("INPUT_DAYS")))
	}
	switch mode {
	case "by_count", "by_days", "regex":
	default:
		errs = append(errs, fmt.Sprintf("参数 mode 必须是 by_count / by_days / regex，当前值: %q", mode))
	}
	if mode == "regex" && regexPattern == "" {
		errs = append(errs, "参数 mode=regex 时必须提供 regex")
	}
	if deleteTagErr != nil {
		errs = append(errs, deleteTagErr.Error())
	}
	if dryRunErr != nil {
		errs = append(errs, dryRunErr.Error())
	}

	var protectRegex, matchRegex *regexp.Regexp
	if protectPattern != "" {
		compiled, err := regexp.Compile(protectPattern)
		if err != nil {
			errs = append(errs, fmt.Sprintf("参数 protect 正则非法: %v", err))
		} else {
			protectRegex = compiled
		}
	}
	if regexPattern != "" {
		compiled, err := regexp.Compile(regexPattern)
		if err != nil {
			errs = append(errs, fmt.Sprintf("参数 regex 正则非法: %v", err))
		} else {
			matchRegex = compiled
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("参数校验失败:\n  - %s", strings.Join(errs, "\n  - "))
	}

	client := &http.Client{Timeout: 60 * time.Second}
	releases, err := fetchAllReleases(client, apiEndpoint, slug, token)
	if err != nil {
		return fmt.Errorf("获取 Release 列表失败: %w", err)
	}
	fmt.Printf("共发现 %d 个 Release\n", len(releases))
	if len(releases) == 0 {
		fmt.Println("没有需要清理的 Release")
		return nil
	}

	toDelete := filterReleases(releases, mode, keep, days, protectRegex, matchRegex)
	if len(toDelete) == 0 {
		fmt.Println("所有版本都在保护范围内，无需清理")
		return nil
	}

	fmt.Printf("\n计划删除 %d 个 Release:\n", len(toDelete))
	for _, rel := range toDelete {
		fmt.Printf("  [%d] %s (%s)\n", rel.ID, rel.TagName, displayTime(rel.CreatedAt))
	}

	if dryRun {
		fmt.Println("\ndry-run 模式，未执行任何删除。设置 dry_run: false 后生效")
		return nil
	}

	success, fail := 0, 0
	var deleteErrs []string
	for _, rel := range toDelete {
		fmt.Printf("\n删除 Release [%d] %s... ", rel.ID, rel.TagName)
		if err := deleteRelease(client, apiEndpoint, slug, token, rel.ID); err != nil {
			fmt.Printf("失败: %v\n", err)
			fail++
			deleteErrs = append(deleteErrs, fmt.Sprintf("Release %s: %v", rel.TagName, err))
			continue
		}
		fmt.Print("Release 已删除")

		if deleteTag && rel.TagName != "" {
			if err := deleteGitTag(client, apiEndpoint, slug, token, rel.TagName); err != nil {
				msg := fmt.Sprintf("Git Tag %s 删除失败: %v", rel.TagName, err)
				fmt.Printf(", %s", msg)
				deleteErrs = append(deleteErrs, msg)
			} else {
				fmt.Print(", Git Tag 已删除")
			}
		}
		fmt.Println()
		success++
	}

	fmt.Printf("\n完成: 成功 %d, 失败 %d\n", success, fail)
	if len(deleteErrs) > 0 {
		return fmt.Errorf("删除失败 %d 项:\n  - %s", len(deleteErrs), strings.Join(deleteErrs, "\n  - "))
	}
	return nil
}

func setGitHubHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
}

func fetchAllReleases(client *http.Client, api, slug, token string) ([]Release, error) {
	var all []Release
	for page := 1; ; page++ {
		endpoint := fmt.Sprintf("%s/repos/%s/releases?page=%d&per_page=%d", api, slug, page, releasesPageSize)
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		setGitHubHeaders(req, token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("第 %d 页请求失败: %w", page, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("第 %d 页读取响应失败: %w", page, readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("第 %d 页 API 返回 %d: %s", page, resp.StatusCode, truncate(string(body), 200))
		}

		var pageReleases []Release
		if err := json.Unmarshal(body, &pageReleases); err != nil {
			return nil, fmt.Errorf("第 %d 页响应解析失败: %w: %s", page, err, truncate(string(body), 200))
		}
		all = append(all, pageReleases...)
		if len(pageReleases) < releasesPageSize {
			break
		}
	}
	return all, nil
}

func filterReleases(releases []Release, mode string, keep, days int, protect, match *regexp.Regexp) []Release {
	sort.Slice(releases, func(i, j int) bool {
		ti, oki := parseCreatedAt(releases[i].CreatedAt)
		tj, okj := parseCreatedAt(releases[j].CreatedAt)
		if oki && okj {
			return ti.After(tj)
		}
		if oki != okj {
			return oki
		}
		return releases[i].CreatedAt > releases[j].CreatedAt
	})

	switch mode {
	case "by_count":
		var count int
		var toDelete []Release
		for _, rel := range releases {
			if protect != nil && protect.MatchString(rel.TagName) {
				continue
			}
			count++
			if count > keep {
				toDelete = append(toDelete, rel)
			}
		}
		return toDelete

	case "by_days":
		cutoff := time.Now().AddDate(0, 0, -days)
		var toDelete []Release
		for _, rel := range releases {
			if protect != nil && protect.MatchString(rel.TagName) {
				continue
			}
			created, ok := parseCreatedAt(rel.CreatedAt)
			if !ok {
				continue
			}
			if created.Before(cutoff) {
				toDelete = append(toDelete, rel)
			}
		}
		return toDelete

	default: // regex
		var toDelete []Release
		for _, rel := range releases {
			if protect != nil && protect.MatchString(rel.TagName) {
				continue
			}
			if match != nil && match.MatchString(rel.TagName) {
				toDelete = append(toDelete, rel)
			}
		}
		return toDelete
	}
}

func deleteRelease(client *http.Client, api, slug, token string, id int64) error {
	endpoint := fmt.Sprintf("%s/repos/%s/releases/%d", api, slug, id)
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	setGitHubHeaders(req, token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("HTTP %d（读取响应失败: %v）", resp.StatusCode, readErr)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 100))
	}
	return nil
}

func deleteGitTag(client *http.Client, api, slug, token, tag string) error {
	endpoint := fmt.Sprintf("%s/repos/%s/git/refs/tags/%s", api, slug, url.PathEscape(tag))
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	setGitHubHeaders(req, token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("HTTP %d（读取响应失败: %v）", resp.StatusCode, readErr)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 100))
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseBoolEnv(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback, fmt.Errorf("参数 %s 必须是布尔值，当前值: %q", strings.ToLower(strings.TrimPrefix(key, "INPUT_")), v)
	}
	return parsed, nil
}

func parseCreatedAt(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func displayTime(value string) string {
	if parsed, ok := parseCreatedAt(value); ok {
		return parsed.Format("2006-01-02")
	}
	if len(value) > 10 {
		return value[:10]
	}
	return value
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
