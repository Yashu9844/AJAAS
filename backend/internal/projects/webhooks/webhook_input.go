package webhooks

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jaas/jaas/internal/projects/kernel"
)

// Limits (resource exhaustion, S17).
const (
	maxWebhooksPerProject     = 20
	maxIntegrationsPerProject = 10
	maxEventTypes             = 30
	disableAfterFailures      = 20
)

// WebhookInput creates a webhook; Format defaults to generic and EventTypes to every event.
type WebhookInput struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Format     string   `json:"format"`
	EventTypes []string `json:"event_types"`
}

// WebhookUpdate edits a webhook; nil fields stay unchanged. Setting Active to true re-enables a disabled webhook.
type WebhookUpdate struct {
	Name       *string   `json:"name"`
	URL        *string   `json:"url"`
	Format     *string   `json:"format"`
	EventTypes *[]string `json:"event_types"`
	Active     *bool     `json:"active"`
}

var eventTypePattern = regexp.MustCompile(`^[a-z][a-z_]*(\.[a-z][a-z_]*){1,3}$`)

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 60 {
		return "", kernel.Invalid("name", "length must be between 1 and 60")
	}
	return name, nil
}

func cleanURL(raw string) (string, error) {
	u, err := ValidateURL(strings.TrimSpace(raw))
	if err != nil {
		return "", kernel.Invalid("url", err.Error())
	}
	return u.String(), nil
}

func cleanFormat(f string) (string, error) {
	switch f {
	case "":
		return FormatGeneric, nil
	case FormatGeneric, FormatSlack, FormatDiscord:
		return f, nil
	}
	return "", kernel.Invalid("format", "must be generic, slack or discord")
}

// cleanEventTypes accepts "*" or routing keys such as task.created; duplicates collapse and "*" absorbs the rest.
func cleanEventTypes(in []string) (kernel.StringList, error) {
	if len(in) == 0 {
		return kernel.StringList{"*"}, nil
	}
	if len(in) > maxEventTypes {
		return nil, kernel.Invalid("event_types", "at most 30 event types")
	}
	seen := map[string]bool{}
	out := kernel.StringList{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "*" {
			return kernel.StringList{"*"}, nil
		}
		if len(t) > 100 || !eventTypePattern.MatchString(t) {
			return nil, kernel.Invalid("event_types", "must be * or event names such as task.created")
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out, nil
}

// build validates a create request into a Webhook (without ids).
func (in WebhookInput) build() (Webhook, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return Webhook{}, err
	}
	u, err := cleanURL(in.URL)
	if err != nil {
		return Webhook{}, err
	}
	format, err := cleanFormat(in.Format)
	if err != nil {
		return Webhook{}, err
	}
	types, err := cleanEventTypes(in.EventTypes)
	if err != nil {
		return Webhook{}, err
	}
	return Webhook{Name: name, URL: u, Format: format, EventTypes: types, Active: true}, nil
}

// apply validates the changes and writes them onto w.
func (in WebhookUpdate) apply(w *Webhook) error {
	var err error
	if in.Name != nil {
		if w.Name, err = cleanName(*in.Name); err != nil {
			return err
		}
	}
	if in.URL != nil {
		if w.URL, err = cleanURL(*in.URL); err != nil {
			return err
		}
	}
	if in.Format != nil {
		if w.Format, err = cleanFormat(*in.Format); err != nil {
			return err
		}
	}
	if in.EventTypes != nil {
		if w.EventTypes, err = cleanEventTypes(*in.EventTypes); err != nil {
			return err
		}
	}
	if in.Active != nil {
		w.Active = *in.Active
		if w.Active {
			w.DisabledAt, w.FailureCount = nil, 0
		}
	}
	return nil
}
