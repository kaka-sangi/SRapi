package service

import "testing"

func TestParseSSEFramesFoldsMultilineData(t *testing.T) {
	body := []byte(": keep-alive\r\n" +
		"retry: 1000\r\n" +
		"event: message\r\n" +
		"data: {\"choices\":[{\"delta\":\r\n" +
		"data: {\"content\":\"multi\"}}]}\r\n" +
		"\r\n" +
		"data: [DONE]")

	frames, err := parseSSEFrames(body)
	if err != nil {
		t.Fatalf("parse SSE frames: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("expected two payload frames, got %+v", frames)
	}
	if frames[0].Event != "message" {
		t.Fatalf("expected event name to be preserved, got %+v", frames[0])
	}
	if want := "{\"choices\":[{\"delta\":\n{\"content\":\"multi\"}}]}"; frames[0].Data != want {
		t.Fatalf("expected multiline data to be folded as %q, got %q", want, frames[0].Data)
	}
	if frames[1].Data != "[DONE]" {
		t.Fatalf("expected unterminated final frame to flush, got %+v", frames[1])
	}
}
