package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

type turnstileOrderedMap struct {
	keys   []string
	values map[string]any
}

func (m *turnstileOrderedMap) add(key string, value any) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

type turnstileFunc = func(args []any)

func solveTurnstileToken(dx string, p string) string {
	decoded, err := base64.StdEncoding.DecodeString(dx)
	if err != nil {
		return ""
	}
	decrypted := xorString(string(decoded), p)
	var tokenList [][]any
	if err := json.Unmarshal([]byte(decrypted), &tokenList); err != nil {
		return ""
	}

	pm := map[float64]any{}
	startTime := time.Now()
	result := ""

	getF := func(args []any, i int) float64 {
		if i >= len(args) {
			return 0
		}
		f, _ := toFloat(args[i])
		return f
	}

	pm[9] = tokenList
	pm[10] = "window"
	pm[16] = p

	pm[1] = turnstileFunc(func(args []any) {
		e, t := getF(args, 0), getF(args, 1)
		pm[e] = xorString(turnstileToStr(pm[e]), turnstileToStr(pm[t]))
	})
	pm[2] = turnstileFunc(func(args []any) {
		if len(args) < 2 {
			return
		}
		e := getF(args, 0)
		pm[e] = args[1]
	})
	pm[3] = turnstileFunc(func(args []any) {
		if len(args) < 1 {
			return
		}
		s := turnstileToStr(args[0])
		result = base64.StdEncoding.EncodeToString([]byte(s))
	})
	pm[5] = turnstileFunc(func(args []any) {
		e, t := getF(args, 0), getF(args, 1)
		current := pm[e]
		incoming := pm[t]
		if currentSlice, ok := current.([]any); ok {
			pm[e] = append(currentSlice, incoming)
			return
		}
		pm[e] = turnstileToStr(current) + turnstileToStr(incoming)
	})
	pm[6] = turnstileFunc(func(args []any) {
		e, t, n := getF(args, 0), getF(args, 1), getF(args, 2)
		tv, _ := pm[t].(string)
		nv, _ := pm[n].(string)
		if tv != "" && nv != "" {
			value := tv + "." + nv
			if value == "window.document.location" {
				pm[e] = "https://chatgpt.com/"
			} else {
				pm[e] = value
			}
		}
	})
	pm[7] = turnstileFunc(func(args []any) {
		if len(args) < 1 {
			return
		}
		e := getF(args, 0)
		target := pm[e]
		values := make([]any, 0, len(args)-1)
		for _, a := range args[1:] {
			f, _ := toFloat(a)
			values = append(values, pm[f])
		}
		targetStr, isStr := target.(string)
		if isStr && targetStr == "window.Reflect.set" && len(values) >= 3 {
			if om, ok := values[0].(*turnstileOrderedMap); ok {
				om.add(fmt.Sprintf("%v", values[1]), values[2])
			}
		} else if fn, ok := target.(turnstileFunc); ok {
			fn(values)
		}
	})
	pm[8] = turnstileFunc(func(args []any) {
		e, t := getF(args, 0), getF(args, 1)
		pm[e] = pm[t]
	})
	pm[14] = turnstileFunc(func(args []any) {
		e, t := getF(args, 0), getF(args, 1)
		s := turnstileToStr(pm[t])
		var parsed any
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			pm[e] = parsed
		}
	})
	pm[15] = turnstileFunc(func(args []any) {
		e, t := getF(args, 0), getF(args, 1)
		b, err := json.Marshal(pm[t])
		if err == nil {
			pm[e] = string(b)
		}
	})
	pm[17] = turnstileFunc(func(args []any) {
		if len(args) < 2 {
			return
		}
		e, t := getF(args, 0), getF(args, 1)
		target, _ := pm[t].(string)
		callArgs := make([]any, 0, len(args)-2)
		for _, a := range args[2:] {
			f, _ := toFloat(a)
			callArgs = append(callArgs, pm[f])
		}
		switch target {
		case "window.performance.now":
			elapsed := time.Since(startTime)
			pm[e] = float64(elapsed.Nanoseconds())/1e6 + rand.Float64()
		case "window.Object.create":
			pm[e] = &turnstileOrderedMap{values: map[string]any{}}
		case "window.Object.keys":
			if len(callArgs) > 0 {
				if arg, ok := callArgs[0].(string); ok && arg == "window.localStorage" {
					pm[e] = []any{
						"STATSIG_LOCAL_STORAGE_INTERNAL_STORE_V4",
						"STATSIG_LOCAL_STORAGE_STABLE_ID",
						"client-correlated-secret",
						"oai/apps/capExpiresAt",
						"oai-did",
						"STATSIG_LOCAL_STORAGE_LOGGING_REQUEST",
						"UiState.isNavigationCollapsed.1",
					}
				}
			}
		case "window.Math.random":
			pm[e] = rand.Float64()
		default:
			if fn, ok := pm[t].(turnstileFunc); ok {
				fn(callArgs)
			}
		}
	})
	pm[18] = turnstileFunc(func(args []any) {
		e := getF(args, 0)
		s := turnstileToStr(pm[e])
		d, err := base64.StdEncoding.DecodeString(s)
		if err == nil {
			pm[e] = string(d)
		}
	})
	pm[19] = turnstileFunc(func(args []any) {
		e := getF(args, 0)
		s := turnstileToStr(pm[e])
		pm[e] = base64.StdEncoding.EncodeToString([]byte(s))
	})
	pm[20] = turnstileFunc(func(args []any) {
		if len(args) < 3 {
			return
		}
		e, t, n := getF(args, 0), getF(args, 1), getF(args, 2)
		if fmt.Sprintf("%v", pm[e]) == fmt.Sprintf("%v", pm[t]) {
			if fn, ok := pm[n].(turnstileFunc); ok {
				fnArgs := make([]any, 0, len(args)-3)
				for _, a := range args[3:] {
					f, _ := toFloat(a)
					fnArgs = append(fnArgs, pm[f])
				}
				fn(fnArgs)
			}
		}
	})
	pm[21] = turnstileFunc(func(_ []any) {})
	pm[23] = turnstileFunc(func(args []any) {
		if len(args) < 2 {
			return
		}
		e, t := getF(args, 0), getF(args, 1)
		if pm[e] != nil {
			if fn, ok := pm[t].(turnstileFunc); ok {
				fnArgs := make([]any, 0, len(args)-2)
				for _, a := range args[2:] {
					f, _ := toFloat(a)
					fnArgs = append(fnArgs, pm[f])
				}
				fn(fnArgs)
			}
		}
	})
	pm[24] = turnstileFunc(func(args []any) {
		e, t, n := getF(args, 0), getF(args, 1), getF(args, 2)
		tv, _ := pm[t].(string)
		nv, _ := pm[n].(string)
		if tv != "" && nv != "" {
			pm[e] = tv + "." + nv
		}
	})

	executed, skipped, recovered := 0, 0, 0
	for _, token := range tokenList {
		if len(token) == 0 {
			continue
		}
		opcode, ok := toFloat(token[0])
		if !ok {
			skipped++
			continue
		}
		fn, ok := pm[opcode].(turnstileFunc)
		if !ok {
			skipped++
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					recovered++
				}
			}()
			fn(token[1:])
			executed++
		}()
	}
	return result
}

func turnstileToStr(value any) string {
	if value == nil {
		return "undefined"
	}
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("%g", v)
	case string:
		specials := map[string]string{
			"window.Math":            "[object Math]",
			"window.Reflect":         "[object Reflect]",
			"window.performance":     "[object Performance]",
			"window.localStorage":    "[object Storage]",
			"window.Object":          "function Object() { [native code] }",
			"window.Reflect.set":     "function set() { [native code] }",
			"window.performance.now": "function () { [native code] }",
			"window.Object.create":   "function create() { [native code] }",
			"window.Object.keys":     "function keys() { [native code] }",
			"window.Math.random":     "function random() { [native code] }",
		}
		if s, ok := specials[v]; ok {
			return s
		}
		return v
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			if s, ok := item.(string); ok {
				parts[i] = s
			} else {
				parts[i] = fmt.Sprintf("%v", item)
			}
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func xorString(text string, key string) string {
	if key == "" {
		return text
	}
	runes := []rune(text)
	keyRunes := []rune(key)
	out := make([]rune, len(runes))
	for i, ch := range runes {
		out[i] = ch ^ keyRunes[i%len(keyRunes)]
	}
	return string(out)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
