package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// localLocation 优先使用 TZ，其次使用系统本地时区，均不可用时回退 UTC。
func localLocation() *time.Location {
	if name := strings.TrimSpace(os.Getenv("TZ")); name != "" {
		if location, err := time.LoadLocation(strings.TrimPrefix(name, ":")); err == nil {
			return location
		}
		return time.UTC
	}
	if time.Local != nil {
		return time.Local
	}
	return time.UTC
}

// 打印当前时间和时区
func printTimeInfo(w io.Writer) {
	location := localLocation()
	now := time.Now().In(location)
	_, _ = fmt.Fprintf(w, "当前时间: %s，时区: %s (%s)\n", now.Format(time.DateTime), location, now.Format("Z07:00"))
}

// parseCreatedAt 按指定时区解析时间，无法解析时返回 false。
func parseCreatedAt(value string, location *time.Location) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	for _, layout := range []string{time.DateTime, time.DateOnly} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// displayTime 将时间转换为指定时区并格式化为日期。
func displayTime(value string, location *time.Location) string {
	if parsed, ok := parseCreatedAt(value, location); ok {
		return parsed.In(location).Format(time.DateOnly)
	}
	if len(value) > 10 {
		return value[:10]
	}
	return value
}
