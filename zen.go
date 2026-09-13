package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type zenUsageWindow struct {
	Status   string  `json:"status"`
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resetsAt"`
}

type zenUsage struct {
	Rolling zenUsageWindow `json:"rolling"`
	Weekly  zenUsageWindow `json:"weekly"`
	Monthly zenUsageWindow `json:"monthly"`
}

type zenUsageResponse struct {
	Usage zenUsage `json:"usage"`
}

func remainingFraction(usedPercent float64) float64 {
	frac := 1 - usedPercent/100
	if frac < 0 {
		return 0
	}
	if frac > 1 {
		return 1
	}
	return frac
}

func zenGet(ctx context.Context, url, apiKey string) ([]byte, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cpa-plugin-opencode-session/"+pluginVersion)
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func fetchZenUsage(ctx context.Context, baseURL, apiKey string) (zenUsage, error) {
	raw, status, err := zenGet(ctx, strings.TrimRight(baseURL, "/")+"/usage", apiKey)
	if err != nil {
		return zenUsage{}, err
	}
	if status >= 400 {
		return zenUsage{}, fmt.Errorf("usage HTTP %d: %s", status, summarizeBody(raw))
	}
	var parsed zenUsageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return zenUsage{}, err
	}
	return parsed.Usage, nil
}

func fetchZenModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	raw, status, err := zenGet(ctx, strings.TrimRight(baseURL, "/")+"/models", apiKey)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("models HTTP %d: %s", status, summarizeBody(raw))
	}
	return parseZenModelIDs(raw)
}

func parseZenModelIDs(raw []byte) ([]string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	items, _ := payload["data"].([]any)
	if items == nil {
		if nested, ok := payload["models"].([]any); ok {
			items = nested
		}
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := obj["id"].(string)
		if id == "" {
			id, _ = obj["name"].(string)
		}
		id = strings.TrimSpace(id)
		id = strings.TrimPrefix(id, "opencode-go/")
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("upstream returned no models")
	}
	return out, nil
}

func summarizeBody(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 240 {
		return s[:240]
	}
	return s
}

func maskKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 10 {
		return "sk-***"
	}
	return key[:6] + "…" + key[len(key)-4:]
}
