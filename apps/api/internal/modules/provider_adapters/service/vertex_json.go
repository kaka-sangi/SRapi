package service

import "encoding/json"

// jsonMarshalForVertex isolates the encoding/json import to one helper so the
// vertex.go integration stays focused on the request rewriting and the
// JSON marshaling is trivially mockable / replaceable.
func jsonMarshalForVertex(v any) ([]byte, error) {
	return json.Marshal(v)
}
