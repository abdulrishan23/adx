package anthropic

import "encoding/json"

func jsonUnmarshal(raw []byte, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}
