package service

import "testing"

func TestChatGPTWebWSFallbackInspectOffered(t *testing.T) {
	m := chatGPTWebWSFallbackMetricsSingleton()
	m.Reset()

	got := ChatGPTWebWSFallbackInspect([]byte(`data: {"conversation_id":"abc","wss_url":"wss://chat.openai.com/conv"}`))
	if !got.Offered || !got.FellBack {
		t.Fatalf("expected Offered+FellBack=true, got %+v", got)
	}
	if chatGPTWebWSFallbackHeaderValue(got) != "1" {
		t.Fatalf("expected header value '1'")
	}
	snap := m.Snapshot()
	if snap.WSOffered != 1 || snap.SSEUsed != 1 {
		t.Fatalf("metrics snapshot mismatch: %+v", snap)
	}
}

func TestChatGPTWebWSFallbackInspectNotOffered(t *testing.T) {
	m := chatGPTWebWSFallbackMetricsSingleton()
	m.Reset()

	got := ChatGPTWebWSFallbackInspect([]byte(`data: {"message":{"author":{"role":"assistant"},"content":{"parts":["hi"]}}}`))
	if got.Offered || got.FellBack {
		t.Fatalf("expected Offered=false, got %+v", got)
	}
	if chatGPTWebWSFallbackHeaderValue(got) != "" {
		t.Fatal("expected empty header value")
	}
	snap := m.Snapshot()
	if snap.SSEUsed != 1 || snap.WSOffered != 0 {
		t.Fatalf("metrics snapshot mismatch: %+v", snap)
	}
}

func TestChatGPTWebWSFallbackRecordAttemptFailed(t *testing.T) {
	m := &ChatGPTWebWSFallbackMetrics{}
	m.recordWSAttemptFailed("dial timeout")
	snap := m.Snapshot()
	if snap.WSAttempts != 1 || snap.WSFailures != 1 {
		t.Fatalf("metrics: %+v", snap)
	}
}
