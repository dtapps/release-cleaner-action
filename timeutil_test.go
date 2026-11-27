package main

import (
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
