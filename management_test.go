package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
}

func TestHandleManagementResourceHTML(t *testing.T) {
	t.Parallel()
	p := &sessionPlugin{cfg: defaultConfig()}
	resp, err := p.HandleManagement(context.Background(), pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/opencode-session/status",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(resp.Body)
	if !strings.Contains(body, "快捷接入") || !strings.Contains(body, "OpenCode Go") {
		t.Fatalf("html = %s", body)
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
