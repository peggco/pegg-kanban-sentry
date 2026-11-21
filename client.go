package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const userAgent = "pegg-kanban-sentry-plugin"

type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient(s Settings) *Client {
	return &Client{
		baseURL: strings.TrimRight(s.BaseURL, "/"),
		token:   strings.TrimSpace(s.Token),
		http:    &http.Client{Timeout: s.timeout()},
	}
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return &APIError{Method: method, Path: path, Status: resp.StatusCode, Body: strings.TrimSpace(string(data))}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

type sentryIssue struct {
	ID          string         `json:"id"`
	ShortID     string         `json:"shortId"`
	Title       string         `json:"title"`
	Culprit     string         `json:"culprit"`
	Permalink   string         `json:"permalink"`
	Status      string         `json:"status"`
	Level       string         `json:"level"`
	Count       string         `json:"count"`
	UserCount   int            `json:"userCount"`
	FirstSeen   string         `json:"firstSeen"`
	LastSeen    string         `json:"lastSeen"`
	Metadata    map[string]any `json:"metadata"`
	IsUnhandled bool           `json:"isUnhandled"`
}

func (c *Client) listIssues(ctx context.Context, org, project, query string, limit int) ([]sentryIssue, error) {
	q := url.Values{}
	if strings.TrimSpace(query) != "" {
		q.Set("query", query)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := fmt.Sprintf("/projects/%s/%s/issues/", url.PathEscape(org), url.PathEscape(project))
	var issues []sentryIssue
	if err := c.do(ctx, http.MethodGet, path, q, nil, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

func (c *Client) commentIssue(ctx context.Context, issueID, text string) error {
	path := fmt.Sprintf("/issues/%s/comments/", url.PathEscape(issueID))
	return c.do(ctx, http.MethodPost, path, nil, map[string]string{"text": text}, nil)
}

func (c *Client) resolveIssue(ctx context.Context, issueID string) error {
	path := fmt.Sprintf("/issues/%s/", url.PathEscape(issueID))
	return c.do(ctx, http.MethodPut, path, nil, map[string]string{"status": "resolved"}, nil)
}
