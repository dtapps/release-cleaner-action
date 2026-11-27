package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestRunCleansReleasesAndTagsIndependentlyWhenCountsDiffer(t *testing.T) {
	releases := []Release{
		{ID: 1, TagName: "v1.0.0", CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: 2, TagName: "v2.0.0", CreatedAt: "2024-02-01T00:00:00Z"},
		{ID: 3, TagName: "v3.0.0", CreatedAt: "2024-03-01T00:00:00Z"},
	}
	tags := []Tag{
		{Name: "v1.0.0", Commit: TagCommit{SHA: "sha1"}},
		{Name: "v2.0.0", Commit: TagCommit{SHA: "sha2"}},
		{Name: "v3.0.0", Commit: TagCommit{SHA: "sha3"}},
		{Name: "v4.0.0", Commit: TagCommit{SHA: "sha4"}},
		{Name: "v5.0.0", Commit: TagCommit{SHA: "sha5"}},
	}

	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/releases":
			writeJSON(t, w, releases)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/tags":
			writeJSON(t, w, tags)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/commits/sha4":
			writeJSON(t, w, map[string]any{"commit": map[string]any{"committer": map[string]any{"date": "2024-04-01T00:00:00Z"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/commits/sha5":
			writeJSON(t, w, map[string]any{"commit": map[string]any{"committer": map[string]any{"date": "2024-05-01T00:00:00Z"}}})
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setRunEnv(t, server.URL, "by_count", "1", "^$", "true", "false")
	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	want := []string{
		"/repos/acme/app/git/refs/tags/v1.0.0",
		"/repos/acme/app/git/refs/tags/v2.0.0",
		"/repos/acme/app/git/refs/tags/v3.0.0",
		"/repos/acme/app/git/refs/tags/v4.0.0",
		"/repos/acme/app/releases/1",
		"/repos/acme/app/releases/2",
	}
	sort.Strings(deleted)
	sort.Strings(want)
	if !reflect.DeepEqual(deleted, want) {
		t.Fatalf("deleted paths = %v, want %v", deleted, want)
	}
}

func TestRunCleansOrphanTagsWhenNoReleases(t *testing.T) {
	tags := []Tag{
		{Name: "v1.0.0", Commit: TagCommit{SHA: "sha1"}},
		{Name: "v2.0.0", Commit: TagCommit{SHA: "sha2"}},
		{Name: "v3.0.0", Commit: TagCommit{SHA: "sha3"}},
	}

	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/releases":
			writeJSON(t, w, []Release{})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/tags":
			writeJSON(t, w, tags)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/acme/app/commits/"):
			sha := strings.TrimPrefix(r.URL.Path, "/repos/acme/app/commits/")
			dates := map[string]string{
				"sha1": "2024-01-01T00:00:00Z",
				"sha2": "2024-02-01T00:00:00Z",
				"sha3": "2024-03-01T00:00:00Z",
			}
			writeJSON(t, w, map[string]any{"commit": map[string]any{"committer": map[string]any{"date": dates[sha]}}})
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setRunEnv(t, server.URL, "by_count", "1", "^$", "true", "false")
	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	want := []string{
		"/repos/acme/app/git/refs/tags/v1.0.0",
		"/repos/acme/app/git/refs/tags/v2.0.0",
	}
	sort.Strings(deleted)
	sort.Strings(want)
	if !reflect.DeepEqual(deleted, want) {
		t.Fatalf("deleted paths = %v, want %v", deleted, want)
	}
}

func TestRunCleansReleasesWhenNoTags(t *testing.T) {
	releases := []Release{
		{ID: 1, TagName: "v1.0.0", CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: 2, TagName: "v2.0.0", CreatedAt: "2024-02-01T00:00:00Z"},
		{ID: 3, TagName: "v3.0.0", CreatedAt: "2024-03-01T00:00:00Z"},
	}

	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/releases":
			writeJSON(t, w, releases)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/tags":
			writeJSON(t, w, []Tag{})
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setRunEnv(t, server.URL, "by_count", "1", "^$", "true", "false")
	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	want := []string{
		"/repos/acme/app/releases/1",
		"/repos/acme/app/releases/2",
	}
	sort.Strings(deleted)
	sort.Strings(want)
	if !reflect.DeepEqual(deleted, want) {
		t.Fatalf("deleted paths = %v, want %v", deleted, want)
	}
}

func setRunEnv(t *testing.T, apiURL, mode, keep, protect, deleteTag, dryRun string) {
	t.Helper()
	t.Setenv("GITHUB_API_URL", apiURL)
	t.Setenv("INPUT_TOKEN", "test-token")
	t.Setenv("GITHUB_REPOSITORY", "acme/app")
	t.Setenv("INPUT_MODE", mode)
	t.Setenv("INPUT_KEEP", keep)
	t.Setenv("INPUT_DAYS", "30")
	t.Setenv("INPUT_REGEX", "")
	t.Setenv("INPUT_PROTECT", protect)
	t.Setenv("INPUT_DELETE_GIT_TAG", deleteTag)
	t.Setenv("INPUT_DRY_RUN", dryRun)
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func Example_filterTags() {
	tags := []Tag{
		{Name: "v1.0.0", CreatedAt: "2024-01-01T00:00:00Z"},
		{Name: "v2.0.0", CreatedAt: "2024-02-01T00:00:00Z"},
		{Name: "v3.0.0", CreatedAt: "2024-03-01T00:00:00Z"},
	}
	for _, tag := range filterTags(tags, "by_count", 1, 0, nil, nil) {
		fmt.Println(tag.Name)
	}
	// Output:
	// v2.0.0
	// v1.0.0
}
