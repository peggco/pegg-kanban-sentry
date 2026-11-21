package main

import (
	"fmt"
	"strconv"
	"strings"
)

func hookIssue(issue sentryIssue) HookIssue {
	number := 0
	if n, err := strconv.Atoi(strings.TrimSpace(issue.ID)); err == nil {
		number = n
	}
	key := strings.TrimSpace(issue.ShortID)
	if key == "" {
		key = issue.ID
	}
	labels := []string{}
	if level := strings.TrimSpace(issue.Level); level != "" {
		labels = append(labels, level)
	}
	if issue.IsUnhandled {
		labels = append(labels, "unhandled")
	}
	return HookIssue{
		ExternalID: issue.ID,
		Number:     number,
		Key:        key,
		Title:      issueTitle(issue),
		Body:       issueBody(issue),
		URL:        issue.Permalink,
		Labels:     labels,
		State:      issue.Status,
		Priority:   issuePriority(issue.Level),
		CreatedAt:  issue.FirstSeen,
		UpdatedAt:  issue.LastSeen,
	}
}

func issueTitle(issue sentryIssue) string {
	if title := strings.TrimSpace(issue.Title); title != "" {
		return title
	}
	if culprit := strings.TrimSpace(issue.Culprit); culprit != "" {
		return culprit
	}
	if key := strings.TrimSpace(issue.ShortID); key != "" {
		return "Sentry issue " + key
	}
	return "Sentry issue"
}

func issueBody(issue sentryIssue) string {
	var b strings.Builder
	if culprit := strings.TrimSpace(issue.Culprit); culprit != "" {
		fmt.Fprintf(&b, "Culprit: %s\n", culprit)
	}
	if level := strings.TrimSpace(issue.Level); level != "" {
		fmt.Fprintf(&b, "Level: %s\n", level)
	}
	if count := strings.TrimSpace(issue.Count); count != "" {
		fmt.Fprintf(&b, "Events: %s (%d users)\n", count, issue.UserCount)
	}
	if issue.IsUnhandled {
		b.WriteString("Unhandled: yes\n")
	}
	if url := strings.TrimSpace(issue.Permalink); url != "" {
		fmt.Fprintf(&b, "URL: %s\n", url)
	}
	return strings.TrimSpace(b.String())
}

func issuePriority(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "fatal", "critical":
		return "urgent"
	case "error":
		return "high"
	case "warning":
		return "medium"
	case "info", "debug":
		return "low"
	default:
		return ""
	}
}
