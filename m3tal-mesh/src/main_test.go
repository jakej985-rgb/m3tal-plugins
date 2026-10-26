package main

import (
	"encoding/json"
	"testing"
)

func TestRPCHandlerInit(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "m3tal.init",
	}
	handleRequest(&req)
}

func TestRPCHandlerCollect(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "m3tal.collect",
	}
	handleRequest(&req)
}

func TestRPCHandlerActionPairAndForget(t *testing.T) {
	pairParams, _ := json.Marshal(map[string]interface{}{
		"action_id": "pair_node",
		"name":      "Test Server",
		"ip":        "192.168.1.199",
		"role":      "server",
	})

	reqPair := RPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "m3tal.action",
		Params:  pairParams,
	}
	handleRequest(&reqPair)

	nodeMutex.RLock()
	var testNodeID string
	for _, n := range nodes {
		if n.IP == "192.168.1.199" {
			testNodeID = n.ID
			break
		}
	}
	nodeMutex.RUnlock()

	if testNodeID == "" {
		t.Fatalf("Expected newly paired node to exist in nodes list")
	}

	forgetParams, _ := json.Marshal(map[string]interface{}{
		"action_id": "forget_node",
		"node_id":   testNodeID,
	})
	reqForget := RPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "m3tal.action",
		Params:  forgetParams,
	}
	handleRequest(&reqForget)
}
