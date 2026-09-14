package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int OpenCodeSessionPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void OpenCodeSessionPluginFree(void*, size_t);
extern void OpenCodeSessionPluginShutdown(void);
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var abiState = struct {
	sync.RWMutex
	host         *C.cliproxy_host_api
	plugin       *sessionPlugin
	shuttingDown bool
	inFlight     sync.WaitGroup
}{}

const maxCGoBytesLen = C.size_t(1<<31 - 1)

type abiEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *abiError       `json:"error,omitempty"`
}

type abiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type abiLifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
	PluginDir  string `json:"plugin_dir,omitempty"`
}

type abiRequestInterceptRequest struct {
	pluginapi.RequestInterceptRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type abiRegistration struct {
	SchemaVersion uint32             `json:"schema_version"`
	Metadata      pluginapi.Metadata `json:"metadata"`
	Capabilities  abiCapabilities    `json:"capabilities"`
}

type abiCapabilities struct {
	RequestInterceptor bool `json:"request_interceptor"`
	ManagementAPI      bool `json:"management_api"`
	QuotaProvider      bool `json:"quota_provider"`
	UsagePlugin        bool `json:"usage_plugin"`
	Scheduler          bool `json:"scheduler"`
}

type identifierResponse struct {
	Identifier string `json:"identifier"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if host == nil || plugin == nil {
		return 1
	}
	abiState.Lock()
	abiState.host = host
	abiState.shuttingDown = false
	abiState.Unlock()

	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.OpenCodeSessionPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.OpenCodeSessionPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.OpenCodeSessionPluginShutdown)
	return 0
}

//export OpenCodeSessionPluginCall
func OpenCodeSessionPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeABIResponse(response, abiErrorEnvelope("invalid_method", "method is required"))
		return 0
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		if requestLen > maxCGoBytesLen {
			writeABIResponse(response, abiErrorEnvelope("request_too_large", "request payload is too large"))
			return 0
		}
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleABIMethod(context.Background(), C.GoString(method), requestBytes)
	if errHandle != nil {
		writeABIResponse(response, abiErrorEnvelope("plugin_error", errHandle.Error()))
		return 0
	}
	writeABIResponse(response, raw)
	return 0
}

//export OpenCodeSessionPluginFree
func OpenCodeSessionPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export OpenCodeSessionPluginShutdown
func OpenCodeSessionPluginShutdown() {
	abiState.Lock()
	abiState.shuttingDown = true
	abiState.plugin = nil
	abiState.host = nil
	abiState.Unlock()
	abiState.inFlight.Wait()
}

func handleABIMethod(ctx context.Context, method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return handleRegister(request)
	}

	p, done, errPlugin := beginPluginCall()
	if errPlugin != nil {
		return nil, errPlugin
	}
	defer done()

	switch method {
	case pluginabi.MethodRequestInterceptBefore:
		var req abiRequestInterceptRequest
		if errDecode := json.Unmarshal(request, &req); errDecode != nil {
			return nil, errDecode
		}
		resp, errCall := p.InterceptRequestBeforeAuth(ctx, req.RequestInterceptRequest)
		return abiOKEnvelopeWithError(resp, errCall)
	case pluginabi.MethodRequestInterceptAfter:
		var req abiRequestInterceptRequest
		if errDecode := json.Unmarshal(request, &req); errDecode != nil {
			return nil, errDecode
		}
		resp, errCall := p.InterceptRequestAfterAuth(ctx, req.RequestInterceptRequest)
		return abiOKEnvelopeWithError(resp, errCall)
	case pluginabi.MethodManagementRegister:
		var req pluginapi.ManagementRegistrationRequest
		_ = json.Unmarshal(request, &req)
		resp, errCall := p.RegisterManagement(ctx, req)
		if errCall != nil {
			return nil, errCall
		}
		return abiOKEnvelope(toABIManagementRegistration(resp))
	case pluginabi.MethodManagementHandle:
		var req pluginapi.ManagementRequest
		if errDecode := json.Unmarshal(request, &req); errDecode != nil {
			return nil, errDecode
		}
		resp, errCall := p.HandleManagement(ctx, req)
		return abiOKEnvelopeWithError(resp, errCall)
	case pluginabi.MethodQuotaIdentifier:
		return abiOKEnvelope(identifierResponse{Identifier: quotaProviderID})
	case pluginabi.MethodQuotaDescribe:
		var req pluginapi.QuotaDescribeRequest
		_ = json.Unmarshal(request, &req)
		resp, errCall := (&quotaAdapter{p: p}).DescribeQuota(ctx, req)
		return abiOKEnvelopeWithError(resp, errCall)
	case pluginabi.MethodQuotaFetch:
		var req pluginapi.QuotaFetchRequest
		if errDecode := json.Unmarshal(request, &req); errDecode != nil {
			return nil, errDecode
		}
		resp, errCall := (&quotaAdapter{p: p}).FetchQuota(ctx, req)
		return abiOKEnvelopeWithError(resp, errCall)
	case pluginabi.MethodQuotaReset:
		var req pluginapi.QuotaResetRequest
		_ = json.Unmarshal(request, &req)
		resp, errCall := (&quotaAdapter{p: p}).ResetQuota(ctx, req)
		return abiOKEnvelopeWithError(resp, errCall)
	case pluginabi.MethodUsageHandle:
		var rec pluginapi.UsageRecord
		if errDecode := json.Unmarshal(request, &rec); errDecode != nil {
			return nil, errDecode
		}
		p.HandleUsage(ctx, rec)
		return abiOKEnvelope(map[string]any{})
	case pluginabi.MethodSchedulerPick:
		var req pluginapi.SchedulerPickRequest
		if errDecode := json.Unmarshal(request, &req); errDecode != nil {
			return nil, errDecode
		}
		resp, errCall := p.Pick(ctx, req)
		return abiOKEnvelopeWithError(resp, errCall)
	default:
		return abiErrorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func handleRegister(request []byte) ([]byte, error) {
	var req abiLifecycleRequest
	if errDecode := json.Unmarshal(request, &req); errDecode != nil {
		return nil, errDecode
	}
	plugin, errBuild := buildPlugin(req.ConfigYAML, req.PluginDir)
	if errBuild != nil {
		return nil, errBuild
	}
	p, ok := plugin.Capabilities.RequestInterceptor.(*sessionPlugin)
	if !ok || p == nil {
		return nil, fmt.Errorf("opencode-session registration returned invalid interceptor")
	}
	abiState.Lock()
	abiState.plugin = p
	abiState.shuttingDown = false
	abiState.Unlock()
	return abiOKEnvelope(abiRegistration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata:      plugin.Metadata,
		Capabilities: abiCapabilities{
			RequestInterceptor: plugin.Capabilities.RequestInterceptor != nil,
			ManagementAPI:      plugin.Capabilities.ManagementAPI != nil,
			QuotaProvider:      plugin.Capabilities.QuotaProvider != nil,
			UsagePlugin:        plugin.Capabilities.UsagePlugin != nil,
			Scheduler:          plugin.Capabilities.Scheduler != nil,
		},
	})
}

func beginPluginCall() (*sessionPlugin, func(), error) {
	abiState.Lock()
	defer abiState.Unlock()
	if abiState.shuttingDown {
		return nil, nil, fmt.Errorf("opencode-session plugin is shutting down")
	}
	if abiState.plugin == nil {
		return nil, nil, fmt.Errorf("opencode-session plugin is not registered")
	}
	abiState.inFlight.Add(1)
	return abiState.plugin, abiState.inFlight.Done, nil
}

func abiOKEnvelopeWithError(v any, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	return abiOKEnvelope(v)
}

func abiOKEnvelope(v any) ([]byte, error) {
	raw, errMarshal := json.Marshal(v)
	if errMarshal != nil {
		return nil, errMarshal
	}
	return json.Marshal(abiEnvelope{OK: true, Result: raw})
}

func abiErrorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(abiEnvelope{OK: false, Error: &abiError{Code: code, Message: message}})
	return raw
}

func writeABIResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
