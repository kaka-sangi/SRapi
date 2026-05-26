package service

import (
	"strings"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
)

const responsesWebSearchCallType = "web_search_call"

func requestUsesHostedWebSearch(req gatewaycontract.CanonicalRequest) bool {
	for _, tool := range req.Tools {
		if isHostedWebSearchTool(tool) {
			return true
		}
	}
	if toolChoice, ok := req.ToolChoice.(map[string]any); ok {
		return isHostedWebSearchTool(toolChoice)
	}
	return false
}

func isHostedWebSearchTool(tool map[string]any) bool {
	if len(tool) == 0 {
		return false
	}
	return isHostedWebSearchType(mapStringAny(tool, "type"))
}

func normalizeHostedWebSearchTool(tool map[string]any) map[string]any {
	normalized := cloneMap(tool)
	if normalized == nil {
		normalized = map[string]any{}
	}
	if strings.TrimSpace(mapStringAny(normalized, "type")) == "" {
		normalized["type"] = "web_search"
	}
	return normalized
}

func isHostedWebSearchBlock(block gatewaycontract.ContentBlock) bool {
	if block.Metadata != nil {
		metadataType := mapStringAny(block.Metadata, "type")
		if strings.TrimSpace(metadataType) != "" {
			return strings.EqualFold(strings.TrimSpace(metadataType), responsesWebSearchCallType) || isHostedWebSearchType(metadataType)
		}
	}
	return isHostedWebSearchName(block.ToolName)
}

func shouldSuppressHostedWebSearchArgumentDelta(state *streamToolCallState, delta string) bool {
	if !isHostedWebSearchBlock(state.Block) {
		return false
	}
	if strings.TrimSpace(delta) == "" {
		return false
	}
	return true
}

func hostedWebSearchStreamDoneGroup(state *streamToolCallState, arguments string) (responseStreamDoneEventGroup, bool) {
	completedBlock := state.completedBlock(arguments)
	if !isHostedWebSearchBlock(completedBlock) {
		return responseStreamDoneEventGroup{}, false
	}
	item := hostedWebSearchOutputItem(completedBlock)
	item["id"] = state.ItemID
	return responseStreamDoneEventGroup{
		OutputIndex: state.OutputIndex,
		Events: []StreamEvent{{
			Event: "response.output_item.done",
			Data: map[string]any{
				"type":         "response.output_item.done",
				"output_index": state.OutputIndex,
				"item":         item,
			},
		}},
	}, true
}

func hostedWebSearchStreamStartEvent(state *streamToolCallState) (StreamEvent, bool) {
	if !isHostedWebSearchBlock(state.Block) {
		return StreamEvent{}, false
	}
	item := hostedWebSearchOutputItem(state.startBlock())
	item["id"] = state.ItemID
	item["status"] = "in_progress"
	return StreamEvent{
		Event: "response.output_item.added",
		Data: map[string]any{
			"type":         "response.output_item.added",
			"output_index": state.OutputIndex,
			"item":         item,
		},
	}, true
}

func hostedWebSearchOutputItem(block gatewaycontract.ContentBlock) map[string]any {
	item := outputBlockProperties(block)
	item["type"] = responsesWebSearchCallType
	status := strings.TrimSpace(mapStringAny(item, "status"))
	if status == "" {
		status = "completed"
	}
	item["status"] = status
	if action, ok := hostedWebSearchAction(block); ok {
		item["action"] = action
	}
	delete(item, "call_id")
	delete(item, "name")
	delete(item, "arguments")
	return item
}

func hostedWebSearchAction(block gatewaycontract.ContentBlock) (map[string]any, bool) {
	if action, ok := block.Metadata["action"].(map[string]any); ok && len(action) > 0 {
		return cloneMap(action), true
	}
	args := parseJSONObject(block.ToolArgumentsJSON)
	if len(args) == 0 {
		return nil, false
	}
	action := cloneMap(args)
	if strings.TrimSpace(mapStringAny(action, "type")) == "" {
		action["type"] = "search"
	}
	return action, true
}

func isHostedWebSearchType(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "web_search" ||
		value == "web_search_preview" ||
		value == "google_search" ||
		value == responsesWebSearchCallType ||
		strings.HasPrefix(value, "web_search_")
}

func isHostedWebSearchName(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "web_search" ||
		value == "web_search_preview" ||
		value == "google_search" ||
		strings.HasPrefix(value, "web_search_")
}
