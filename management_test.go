package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestManagementRegisterABIOmitsHandler(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	resp, err := p.RegisterManagement(context.Background(), pluginapi.ManagementRegistrationRequest{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(toABIManagementRegistration(resp))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Handler") {
		t.Fatalf("ABI payload must not include Handler: %s", raw)
	}
	var decoded struct {
		Routes    []pluginapi.ManagementRoute `json:"routes"`
		Resources []pluginapi.ResourceRoute   `json:"resources"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Routes) == 0 || len(decoded.Resources) == 0 {
		t.Fatalf("decoded = %#v payload = %s", decoded, raw)
	}
	foundAuthFiles := false
	for _, route := range decoded.Routes {
		if route.Path == "/plugins/opencode-session/auth-files" && route.Method == http.MethodGet {
			foundAuthFiles = true
			break
		}
	}
	if !foundAuthFiles {
		t.Fatalf("missing GET auth-files route: %#v", decoded.Routes)
	}
}

func TestHandleManagementResourceHTML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 18317\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	resp, err := p.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/opencode-session/status",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(resp.Body)
	if !strings.Contains(body, "订阅池") || !strings.Contains(body, "OpenCode Go") {
		t.Fatalf("html = %s", body)
	}
	if !strings.Contains(body, `window.__OPENCODE_STATUS__ = {`) {
		t.Fatal("expected embedded status JSON")
	}
	if !strings.Contains(body, `"ok":true`) {
		t.Fatalf("embedded status = %s", body)
	}
	if !strings.Contains(body, "typeof o==='string'") {
		t.Fatal("page JS must unwrap JSON-string management keys")
	}
	if !strings.Contains(body, "cli-proxy-auth") {
		t.Fatal("page JS must read CPA persist key cli-proxy-auth")
	}
	if !strings.Contains(body, "__CPA_MGMT_KEY") {
		t.Fatal("page JS must read parent window.__CPA_MGMT_KEY")
	}
	if !strings.Contains(body, "keyFromFiber") {
		t.Fatal("page JS must walk parent React fiber for in-memory managementKey")
	}
	if !strings.Contains(body, "cpa:managementKey") {
		t.Fatal("page JS must read sessionStorage cpa:managementKey")
	}
}

func TestHandleConnectRequiresAPIKey(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	resp, err := p.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/opencode-session/connect",
		Body:   []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.StatusCode, resp.Body)
	}
}

func TestHandleConnectWritesProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-live-test" && r.Header.Get("Authorization") != "Bearer sk-live-two" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			_, _ = w.Write([]byte(`{"data":[{"id":"minimax-m3"}]}`))
		case strings.HasSuffix(r.URL.Path, "/usage"):
			_, _ = w.Write([]byte(`{"usage":{"rolling":{"status":"ok","percent":11,"resetsAt":"t1"},"weekly":{"status":"ok","percent":22,"resetsAt":"t2"},"monthly":{"status":"ok","percent":33,"resetsAt":"t3"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	p.cfg.BaseURL = srv.URL + "/v1"
	resp, err := p.handleConnect(context.Background(), []byte(`{"api_key":"sk-live-test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, resp.Body)
	}
	keys, models, err := listConfiguredKeys(path, "opencode-go")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "sk-live-test" || len(models) != 1 {
		t.Fatalf("keys=%#v models=%#v", keys, models)
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["pool_size"] != float64(1) {
		t.Fatalf("pool_size = %#v body = %s", payload["pool_size"], resp.Body)
	}
	resp2, err := p.handleConnect(context.Background(), []byte(`{"api_key":"sk-live-two"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second connect status = %d body = %s", resp2.StatusCode, resp2.Body)
	}
	keys, _, err = listConfiguredKeys(path, "opencode-go")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("pool keys = %#v", keys)
	}
}

func TestHandleStatusEmptyConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 18317\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	resp, err := p.handleStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, resp.Body)
	}
	if strings.Contains(strings.ToLower(string(resp.Body)), "sk-") && strings.Contains(string(resp.Body), "api_key") {
		t.Fatalf("status leaked a key: %s", resp.Body)
	}
}

func TestHandleAuthFilesListsConfiguredKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	src := []byte("openai-compatibility:\n  - name: opencode-go\n    base-url: https://opencode.ai/zen/go/v1\n    api-key-entries:\n      - api-key: sk-aaaaaaaaaaaaaaaaaaaa\n        proxy-url: direct\n      - api-key: sk-bbbbbbbbbbbbbbbbbbbb\n        proxy-url: direct\n    models:\n      - name: minimax-m3\n      - name: kimi-k3\n")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	firstID := stableCompatAuthID("opencode-go", "sk-aaaaaaaaaaaaaaaaaaaa", "https://opencode.ai/zen/go/v1", "direct")
	p.quotaByAuthID = map[string]cachedQuota{
		firstID: {ExhaustedUntil: time.Now().Add(time.Hour)},
	}

	resp, err := p.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/opencode-session/auth-files",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, resp.Body)
	}
	body := string(resp.Body)
	if strings.Contains(body, "sk-aaaaaaaaaaaaaaaaaaaa") || strings.Contains(body, "sk-bbbbbbbbbbbbbbbbbbbb") {
		t.Fatalf("auth-files leaked a key: %s", body)
	}
	var payload struct {
		OK    bool `json:"ok"`
		Files []struct {
			ID            string           `json:"id"`
			Name          string           `json:"name"`
			Type          string           `json:"type"`
			Provider      string           `json:"provider"`
			Email         string           `json:"email"`
			Status        string           `json:"status"`
			StatusMessage string           `json:"status_message"`
			RuntimeOnly   bool             `json:"runtime_only"`
			Disabled      bool             `json:"disabled"`
			SupportsQuota bool             `json:"supports_quota"`
			Models        []map[string]any `json:"models"`
		} `json:"files"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Files) != 2 {
		t.Fatalf("payload = %#v", payload)
	}
	first, second := payload.Files[0], payload.Files[1]
	wantFirst := opencodeAuthFileName(configuredEntry{
		APIKey: "sk-aaaaaaaaaaaaaaaaaaaa", BaseURL: "https://opencode.ai/zen/go/v1", ProxyURL: "direct",
	}, "opencode-go")
	wantSecond := opencodeAuthFileName(configuredEntry{
		APIKey: "sk-bbbbbbbbbbbbbbbbbbbb", BaseURL: "https://opencode.ai/zen/go/v1", ProxyURL: "direct",
	}, "opencode-go")
	if first.Name != wantFirst || second.Name != wantSecond {
		t.Fatalf("names = %q %q want %q %q", first.Name, second.Name, wantFirst, wantSecond)
	}
	if first.Type != "opencode-go" || first.Provider != "opencode-go" || !first.RuntimeOnly || first.Disabled {
		t.Fatalf("first = %#v", first)
	}
	if first.ID != firstID {
		t.Fatalf("id = %q want %q", first.ID, firstID)
	}
	if first.StatusMessage != "quota exhausted" {
		t.Fatalf("status_message = %q", first.StatusMessage)
	}
	if second.StatusMessage != "" {
		t.Fatalf("second status_message = %q", second.StatusMessage)
	}
	if !first.SupportsQuota || len(first.Models) != 2 {
		t.Fatalf("models = %#v", first.Models)
	}
	if first.Models[0]["id"] != "minimax-m3" || first.Models[1]["id"] != "kimi-k3" {
		t.Fatalf("model ids = %#v", first.Models)
	}
	if first.Email == "" || strings.Contains(first.Email, "aaaaaaaa") {
		t.Fatalf("email = %q", first.Email)
	}
}

func TestHandleAuthFilesEmptyConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("port: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	resp, err := p.handleAuthFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		OK    bool             `json:"ok"`
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Files) != 0 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHandleAuthFilesStatusAndDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	src := []byte("openai-compatibility:\n  - name: other\n    base-url: https://example.com\n    api-key-entries:\n      - api-key: sk-keep\n  - name: opencode-go\n    base-url: https://opencode.ai/zen/go/v1\n    disable-cooling: true\n    api-key-entries:\n      - api-key: sk-aaaaaaaaaaaaaaaaaaaa\n        proxy-url: direct\n      - api-key: sk-bbbbbbbbbbbbbbbbbbbb\n        proxy-url: direct\n")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	p := &sessionPlugin{cfg: defaultConfig()}
	p.cfg.CPAConfigPath = path
	first := opencodeAuthFileName(configuredEntry{
		APIKey: "sk-aaaaaaaaaaaaaaaaaaaa", BaseURL: "https://opencode.ai/zen/go/v1", ProxyURL: "direct",
	}, "opencode-go")

	resp, err := p.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: http.MethodPatch,
		Path:   "/v0/management/plugins/opencode-session/auth-files/status",
		Body:   []byte(`{"name":"` + first + `","disabled":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, resp.Body)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "disabled: true") || !strings.Contains(text, "weight: 0") {
		t.Fatalf("disable did not persist:\n%s", text)
	}
	if !strings.Contains(text, "name: other") || !strings.Contains(text, "sk-keep") {
		t.Fatalf("clobbered unrelated provider:\n%s", text)
	}
	entries, _, err := listConfiguredEntries(path, "opencode-go")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || !entries[0].Disabled || entries[1].Disabled {
		t.Fatalf("entries = %#v", entries)
	}

	resp, err = p.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: http.MethodPatch,
		Path:   "/v0/management/plugins/opencode-session/auth-files/status",
		Body:   []byte(`{"name":"` + first + `","disabled":false}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-enable status = %d body = %s", resp.StatusCode, resp.Body)
	}

	resp, err = p.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: http.MethodDelete,
		Path:   "/v0/management/plugins/opencode-session/auth-files",
		Body:   []byte(`{"names":["` + first + `"]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", resp.StatusCode, resp.Body)
	}
	if strings.Contains(string(resp.Body), "sk-aaaaaaaaaaaaaaaaaaaa") {
		t.Fatalf("delete leaked a key: %s", resp.Body)
	}
	entries, _, err = listConfiguredEntries(path, "opencode-go")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].APIKey != "sk-bbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("after delete entries = %#v", entries)
	}
	raw, _ = os.ReadFile(path)
	if !strings.Contains(string(raw), "name: other") {
		t.Fatalf("deleted unrelated provider:\n%s", raw)
	}

	second := opencodeAuthFileName(entries[0], "opencode-go")
	resp, err = p.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: http.MethodDelete,
		Path:   "/v0/management/plugins/opencode-session/auth-files",
		Body:   []byte(`{"name":"` + second + `"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete last status = %d body = %s", resp.StatusCode, resp.Body)
	}
	entries, _, err = listConfiguredEntries(path, "opencode-go")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected provider removed, entries = %#v", entries)
	}
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), "name: opencode-go") {
		t.Fatalf("opencode-go channel still present:\n%s", raw)
	}
	if !strings.Contains(string(raw), "name: other") {
		t.Fatalf("lost unrelated provider:\n%s", raw)
	}
}

func TestIsOpenCodeQuotaRequest(t *testing.T) {
	t.Parallel()
	ok := isOpenCodeQuotaRequest(pluginapi.QuotaFetchRequest{
		Provider:   "openai-compatibility",
		Attributes: map[string]string{"compat_name": "opencode-go"},
	}, "opencode-go")
	if !ok {
		t.Fatal("expected opencode-go compat credential")
	}
	if isOpenCodeQuotaRequest(pluginapi.QuotaFetchRequest{
		Provider:   "openai-compatibility",
		Attributes: map[string]string{"compat_name": "other"},
	}, "opencode-go") {
		t.Fatal("other compat provider must not match")
	}
}
