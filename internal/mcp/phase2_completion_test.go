package mcp

import (
	"encoding/json"
	"testing"
)

// ─── Additional MCP Tests (Phase 2 Completion) ─────────────────────────────

func TestGateway_ResourcesRead_RegionsGlobal(t *testing.T) {
	gw := newTestGateway(t)

	req := makeP2Request(t, 10, "resources/read", map[string]string{
		"uri": "tutu://regions/global",
	})

	resp := gw.HandleRequest(req)
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	if result.Contents[0].URI != "tutu://regions/global" {
		t.Errorf("expected tutu://regions/global, got %s", result.Contents[0].URI)
	}

	// Parse the region data
	var regionData map[string]interface{}
	if err := json.Unmarshal([]byte(result.Contents[0].Text), &regionData); err != nil {
		t.Fatalf("parse region data: %v", err)
	}
	if regionData["total_regions"] != float64(3) {
		t.Errorf("expected 3 regions, got %v", regionData["total_regions"])
	}
	regions := regionData["regions"].([]interface{})
	if len(regions) != 3 {
		t.Errorf("expected 3 region entries, got %d", len(regions))
	}
}

func TestGateway_AllThreeResources(t *testing.T) {
	gw := newTestGateway(t)

	// List resources
	req := makeP2Request(t, 20, "resources/list", nil)
	resp := gw.HandleRequest(req)
	if resp == nil || resp.Error != nil {
		t.Fatal("resources/list failed")
	}

	var list struct {
		Resources []struct {
			URI string `json:"uri"`
		} `json:"resources"`
	}
	json.Unmarshal(resp.Result, &list)

	if len(list.Resources) != 3 {
		t.Fatalf("expected 3 resources, got %d", len(list.Resources))
	}

	// Verify all 3 resources are readable
	uris := []string{"tutu://capacity", "tutu://models", "tutu://regions/global"}
	for _, uri := range uris {
		req := makeP2Request(t, 30, "resources/read", map[string]string{"uri": uri})
		resp := gw.HandleRequest(req)
		if resp == nil || resp.Error != nil {
			t.Errorf("resource %s: read failed", uri)
		}
	}
}

func TestGateway_SLATiers_AllFour(t *testing.T) {
	sla := NewSLAEngine()
	tiers := sla.AllTiers()

	if len(tiers) != 4 {
		t.Fatalf("expected 4 SLA tiers, got %d", len(tiers))
	}

	// Verify tier order (highest priority first)
	if tiers[0].Priority != 255 {
		t.Errorf("first tier should be realtime (pri 255), got %d", tiers[0].Priority)
	}
	if tiers[3].Priority != 1 {
		t.Errorf("last tier should be spot (pri 1), got %d", tiers[3].Priority)
	}
}

// makeP2Request creates a JSON-RPC request body (unique name to avoid collision).
func makeP2Request(t *testing.T, id int, method string, params interface{}) []byte {
	t.Helper()
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return data
}
