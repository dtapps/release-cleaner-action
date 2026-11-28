package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestLocalLocationUsesTimezoneOrFallsBackToUTC(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	if got := localLocation().String(); got != "Asia/Shanghai" {
		t.Fatalf("localLocation() = %q", got)
	}

	t.Setenv("TZ", "UTC")
	if got := localLocation(); got != time.UTC {
		t.Fatalf("localLocation() = %q, want UTC", got)
	}

	t.Setenv("TZ", "Invalid/Timezone")
	if got := localLocation(); got != time.UTC {
		t.Fatalf("localLocation() = %q, want UTC", got)
	}
}

func TestDisplayTimeUsesLocalTimezone(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if got := displayTime("2024-01-01T20:00:00Z", location); got != "2024-01-02" {
		t.Fatalf("displayTime() = %q, want 2024-01-02", got)
	}
	if got := displayTime("2024-01-01", location); got != "2024-01-01" {
		t.Fatalf("displayTime() = %q, want 2024-01-01", got)
	}
}

func TestPrintTimeInfo(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().In(location).Add(-time.Second)
	var output bytes.Buffer
	printTimeInfo(&output)
	after := time.Now().In(location).Add(time.Second)

	const prefix = "当前时间: "
	const suffix = "，时区: Asia/Shanghai (+08:00)"
	line := strings.TrimSpace(output.String())
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		t.Fatalf("output = %q, want current time and Asia/Shanghai (+08:00)", output.String())
	}
	printed, err := time.ParseInLocation(time.DateTime, strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix), location)
	if err != nil {
		t.Fatalf("parse printed time %q: %v", line, err)
	}
	if printed.Before(before) || printed.After(after) {
		t.Fatalf("printed time = %s, want between %s and %s", printed, before, after)
	}
}
