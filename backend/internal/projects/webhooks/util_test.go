package webhooks

import (
	"context"
	"encoding/json"
)

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
