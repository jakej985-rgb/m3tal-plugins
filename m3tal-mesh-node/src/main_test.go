package main

import (
	"encoding/json"
	"testing"
)

func TestNodeInit(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "m3tal.init",
	}
	handleRequest(&req)
}

func TestNodeCollect(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "m3tal.collect",
	}
	handleRequest(&req)
}

func TestNodeAction(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{
		"action_id": "beacon_ping",
	})
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "m3tal.action",
		Params:  params,
	}
	handleRequest(&req)
}
