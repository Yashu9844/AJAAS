package kernel

import (
	"context"
	"regexp"
)

// AgentHeader names the request header an AI agent (or any automation acting for a user) sets to identify itself. The user
// token still decides what is allowed; the header only labels the action so activity, events and audit can show it.
const AgentHeader = "X-JAAS-Agent"

type agentKey struct{}

var agentName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,39}$`)

// ValidAgent reports whether name is an acceptable agent label (lowercase, 1-40 characters).
func ValidAgent(name string) bool { return agentName.MatchString(name) }

// WithAgent returns a context that carries the agent label.
func WithAgent(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, agentKey{}, name)
}

// AgentFrom returns the agent label of the context ("" for a person acting directly).
func AgentFrom(ctx context.Context) string {
	name, _ := ctx.Value(agentKey{}).(string)
	return name
}
