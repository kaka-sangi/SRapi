package service

import (
	"bufio"
	"bytes"
	"strings"
)

type sseFrame struct {
	Event string
	Data  string
}

func (f sseFrame) EventType(payloadType string) string {
	return firstNonEmpty(strings.TrimSpace(payloadType), strings.TrimSpace(f.Event))
}

// parseSSEFrames folds repeated data fields into one event payload. Comments
// and event-only frames are metadata-only and do not produce payload frames.
func parseSSEFrames(body []byte) ([]sseFrame, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	frames := make([]sseFrame, 0)
	var current sseFrame
	var dataLines []string
	flush := func() {
		if len(dataLines) == 0 {
			current = sseFrame{}
			return
		}
		current.Data = strings.Join(dataLines, "\n")
		frames = append(frames, current)
		current = sseFrame{}
		dataLines = nil
	}
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			field = line
			value = ""
		}
		if strings.HasPrefix(value, " ") {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "event":
			current.Event = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()
	return frames, nil
}
