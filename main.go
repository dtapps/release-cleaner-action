package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v93/github"
)

type Release struct {
	ID        int64  `json:"id"`
	TagName   string `json:"tag_name"`
	CreatedAt string `json:"created_at"`
}

type Tag struct {
	Name      string    `json:"name"`
	Commit    TagCommit `json:"commit"`
	CreatedAt string    `json:"-"`
}

type TagCommit struct {
	SHA string `json:"sha"`
}

// main 启动清理任务，并在失败时输出错误并退出。
func main() {
	printTimeInfo(os.Stdout)
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

// run 校验参数，筛选并清理 Release 与可选的 Git Tag。
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
	owner, repo, validSlug := strings.Cut(slug, "/")
	if !validSlug || owner == "" || repo == "" || strings.Contains(repo, "/") {
		errs = append(errs, fmt.Sprintf("环境变量 GITHUB_REPOSITORY 必须是 owner/repo 格式，当前值: %q", slug))
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

	location := localLocation()
	fmt.Printf("使用时区: %s\n", location)

	ctx := context.Background()
	client, err := github.NewClient(
		github.WithTimeout(60*time.Second),
		github.WithAuthToken(token),
		github.WithURLs(&apiEndpoint, nil),
	)
	if err != nil {
		return fmt.Errorf("初始化 GitHub 客户端失败: %w", err)
	}

	releases, err := fetchAllReleases(ctx, client, owner, repo)
	if err != nil {
		return fmt.Errorf("获取 Release 列表失败: %w", err)
	}
	fmt.Printf("共发现 %d 个 Release\n", len(releases))

	var tags []Tag
	if deleteTag {
		tags, err = fetchAllTags(ctx, client, owner, repo)
		if err != nil {
			return fmt.Errorf("获取 Git Tag 列表失败: %w", err)
		}
		fmt.Printf("共发现 %d 个 Git Tag\n", len(tags))
		if mode != "regex" {
			if err := populateTagCreatedAt(ctx, client, owner, repo, tags, releases, location, protectRegex); err != nil {
				return fmt.Errorf("获取 Git Tag 时间失败: %w", err)
			}
		}
	}

	toDeleteReleases := filterReleases(releases, mode, keep, days, location, protectRegex, matchRegex)
	var toDeleteTags []Tag
	if deleteTag {
		toDeleteTags = filterTags(tags, mode, keep, days, location, protectRegex, matchRegex)
	}

	if len(toDeleteReleases) == 0 && len(toDeleteTags) == 0 {
		if !deleteTag {
			if len(releases) == 0 {
				fmt.Println("没有需要清理的 Release")
			} else {
				fmt.Println("所有版本都在保护范围内，无需清理")
			}
			return nil
		}
		if len(releases) == 0 && len(tags) == 0 {
			fmt.Println("没有需要清理的 Release 或 Git Tag")
		} else {
			fmt.Println("所有版本都在保护范围内或无需清理")
		}
		return nil
	}

	if len(toDeleteReleases) > 0 {
		fmt.Printf("\n计划删除 %d 个 Release:\n", len(toDeleteReleases))
		for _, rel := range toDeleteReleases {
			fmt.Printf("  [%d] %s (%s)\n", rel.ID, rel.TagName, displayTime(rel.CreatedAt, location))
		}
	}
	if len(toDeleteTags) > 0 {
		fmt.Printf("\n计划删除 %d 个 Git Tag:\n", len(toDeleteTags))
		for _, tag := range toDeleteTags {
			fmt.Printf("  %s (%s)\n", tag.Name, displayTime(tag.CreatedAt, location))
		}
	}

	if dryRun {
		fmt.Println("\ndry-run 模式，未执行任何删除。设置 dry_run: false 后生效")
		return nil
	}

	var deleteErrs []string
	releaseSuccess, releaseFail := 0, 0
	for _, rel := range toDeleteReleases {
		fmt.Printf("\n删除 Release [%d] %s... ", rel.ID, rel.TagName)
		if err := deleteRelease(ctx, client, owner, repo, rel.ID); err != nil {
			fmt.Printf("失败: %v\n", err)
			releaseFail++
			deleteErrs = append(deleteErrs, fmt.Sprintf("Release %s: %v", rel.TagName, err))
			continue
		}
		fmt.Println("Release 已删除")
		releaseSuccess++
	}

	tagSuccess, tagFail := 0, 0
	for _, tag := range toDeleteTags {
		fmt.Printf("\n删除 Git Tag %s... ", tag.Name)
		if err := deleteGitTag(ctx, client, owner, repo, tag.Name); err != nil {
			fmt.Printf("失败: %v\n", err)
			tagFail++
			deleteErrs = append(deleteErrs, fmt.Sprintf("Git Tag %s: %v", tag.Name, err))
			continue
		}
		fmt.Println("Git Tag 已删除")
		tagSuccess++
	}

	fmt.Printf("\n完成: Release 成功 %d, 失败 %d; Git Tag 成功 %d, 失败 %d\n", releaseSuccess, releaseFail, tagSuccess, tagFail)
	if len(deleteErrs) > 0 {
		return fmt.Errorf("删除失败 %d 项:\n  - %s", len(deleteErrs), strings.Join(deleteErrs, "\n  - "))
	}
	return nil
}

// fetchAllReleases 分页获取仓库的全部 Release。
func fetchAllReleases(ctx context.Context, client *github.Client, owner, repo string) ([]Release, error) {
	var all []Release
	opts := &github.ListOptions{Page: 1, PerPage: 100}
	for {
		pageReleases, resp, err := client.Repositories.ListReleases(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		for _, rel := range pageReleases {
			all = append(all, Release{
				ID:        rel.GetID(),
				TagName:   rel.GetTagName(),
				CreatedAt: rel.GetCreatedAt().Format(time.RFC3339),
			})
		}
		if resp.NextPage == 0 {
			return all, nil
		}
		opts.Page = resp.NextPage
	}
}

// fetchAllTags 分页获取仓库的全部 Git Tag。
func fetchAllTags(ctx context.Context, client *github.Client, owner, repo string) ([]Tag, error) {
	var all []Tag
	opts := &github.ListOptions{Page: 1, PerPage: 100}
	for {
		pageTags, resp, err := client.Repositories.ListTags(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		for _, tag := range pageTags {
			all = append(all, Tag{
				Name:   tag.GetName(),
				Commit: TagCommit{SHA: tag.GetCommit().GetSHA()},
			})
		}
		if resp.NextPage == 0 {
			return all, nil
		}
		opts.Page = resp.NextPage
	}
}

// populateTagCreatedAt 补充 Tag 时间，优先复用 Release 时间，否则查询提交时间。
func populateTagCreatedAt(ctx context.Context, client *github.Client, owner, repo string, tags []Tag, releases []Release, location *time.Location, protect *regexp.Regexp) error {
	releaseDates := make(map[string]string, len(releases))
	for _, rel := range releases {
		if rel.TagName != "" {
			releaseDates[rel.TagName] = rel.CreatedAt
		}
	}

	commitDates := make(map[string]string)
	for i := range tags {
		tag := &tags[i]
		if protect != nil && protect.MatchString(tag.Name) {
			continue
		}
		if createdAt, ok := releaseDates[tag.Name]; ok {
			if _, ok := parseCreatedAt(createdAt, location); ok {
				tag.CreatedAt = createdAt
				continue
			}
		}
		if tag.Commit.SHA == "" {
			return fmt.Errorf("Git Tag %s 缺少 commit SHA", tag.Name)
		}
		createdAt, ok := commitDates[tag.Commit.SHA]
		if !ok {
			var err error
			createdAt, err = fetchCommitCreatedAt(ctx, client, owner, repo, tag.Commit.SHA)
			if err != nil {
				return fmt.Errorf("Git Tag %s: %w", tag.Name, err)
			}
			commitDates[tag.Commit.SHA] = createdAt
		}
		tag.CreatedAt = createdAt
	}
	return nil
}

// fetchCommitCreatedAt 获取提交时间，优先使用 committer 时间。
func fetchCommitCreatedAt(ctx context.Context, client *github.Client, owner, repo, sha string) (string, error) {
	commit, _, err := client.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return "", err
	}
	if date := commit.GetCommit().GetCommitter().GetDate(); !date.IsZero() {
		return date.Format(time.RFC3339), nil
	}
	if date := commit.GetCommit().GetAuthor().GetDate(); !date.IsZero() {
		return date.Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("提交 %s 缺少时间信息", sha)
}

// filterReleases 按清理模式筛选待删除的 Release，并跳过受保护版本。
func filterReleases(releases []Release, mode string, keep, days int, location *time.Location, protect, match *regexp.Regexp) []Release {
	sort.Slice(releases, func(i, j int) bool {
		ti, oki := parseCreatedAt(releases[i].CreatedAt, location)
		tj, okj := parseCreatedAt(releases[j].CreatedAt, location)
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
		cutoff := time.Now().In(location).AddDate(0, 0, -days)
		var toDelete []Release
		for _, rel := range releases {
			if protect != nil && protect.MatchString(rel.TagName) {
				continue
			}
			created, ok := parseCreatedAt(rel.CreatedAt, location)
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

// filterTags 按清理模式独立筛选待删除的 Git Tag，并跳过受保护版本。
func filterTags(tags []Tag, mode string, keep, days int, location *time.Location, protect, match *regexp.Regexp) []Tag {
	sort.Slice(tags, func(i, j int) bool {
		ti, oki := parseCreatedAt(tags[i].CreatedAt, location)
		tj, okj := parseCreatedAt(tags[j].CreatedAt, location)
		if oki && okj {
			return ti.After(tj)
		}
		if oki != okj {
			return oki
		}
		return tags[i].CreatedAt > tags[j].CreatedAt
	})

	switch mode {
	case "by_count":
		var count int
		var toDelete []Tag
		for _, tag := range tags {
			if protect != nil && protect.MatchString(tag.Name) {
				continue
			}
			count++
			if count > keep {
				toDelete = append(toDelete, tag)
			}
		}
		return toDelete

	case "by_days":
		cutoff := time.Now().In(location).AddDate(0, 0, -days)
		var toDelete []Tag
		for _, tag := range tags {
			if protect != nil && protect.MatchString(tag.Name) {
				continue
			}
			created, ok := parseCreatedAt(tag.CreatedAt, location)
			if !ok {
				continue
			}
			if created.Before(cutoff) {
				toDelete = append(toDelete, tag)
			}
		}
		return toDelete

	default: // regex
		var toDelete []Tag
		for _, tag := range tags {
			if protect != nil && protect.MatchString(tag.Name) {
				continue
			}
			if match != nil && match.MatchString(tag.Name) {
				toDelete = append(toDelete, tag)
			}
		}
		return toDelete
	}
}

// deleteRelease 删除指定 ID 的 Release。
func deleteRelease(ctx context.Context, client *github.Client, owner, repo string, id int64) error {
	_, err := client.Repositories.DeleteRelease(ctx, owner, repo, id)
	return err
}

// deleteGitTag 删除指定名称的 Git Tag 引用。
func deleteGitTag(ctx context.Context, client *github.Client, owner, repo, tag string) error {
	_, err := client.Git.DeleteRef(ctx, owner, repo, "tags/"+tag)
	return err
}

// getEnv 读取环境变量，值为空时返回默认值。
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseBoolEnv 解析布尔型环境变量，值为空时返回默认值。
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
