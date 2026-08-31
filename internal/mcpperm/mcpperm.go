// Package mcpperm provides simple permission policy and audit utilities for
// MCP tool calls.
package mcpperm

import (
	"strings"
	"sync"
	"time"
)

// Mode controls how a tool call is handled by the policy.
type Mode string

const (
	// ModeAuto allows a tool call without confirmation.
	ModeAuto Mode = "auto"
	// ModeConfirm requires confirmation before a tool call is allowed.
	ModeConfirm Mode = "confirm"
	// ModeDeny forbids a tool call.
	ModeDeny Mode = "deny"
)

// Rule associates a tool pattern with a permission mode.
type Rule struct {
	Tool string
	Mode Mode
	Note string
}

// Policy resolves permission modes for tool names.
type Policy struct {
	Default Mode
	Rules   []Rule
}

// NewPolicy returns a policy with the provided default mode.
func NewPolicy(defaultMode Mode) *Policy {
	return &Policy{Default: defaultMode}
}

// Set adds or replaces the rule for tool. Exact tool names and prefix
// patterns such as "group.*" are both supported.
func (p *Policy) Set(tool string, mode Mode) {
	if p == nil {
		return
	}
	for i := range p.Rules {
		if p.Rules[i].Tool == tool {
			p.Rules[i].Mode = mode
			return
		}
	}
	p.Rules = append(p.Rules, Rule{Tool: tool, Mode: mode})
}

// Mode returns the mode for tool using exact match first, then a prefix
// match for a "group.*" style rule, then the policy default.
func (p *Policy) Mode(tool string) Mode {
	if p == nil {
		return ModeDeny
	}
	for _, r := range p.Rules {
		if r.Tool == tool {
			return r.Mode
		}
	}
	best := ""
	bestMode := p.Default
	for _, r := range p.Rules {
		if strings.HasSuffix(r.Tool, ".*") && strings.HasPrefix(tool, strings.TrimSuffix(r.Tool, "*")) && len(r.Tool) > len(best) {
			best = r.Tool
			bestMode = r.Mode
		}
	}
	if best != "" {
		return bestMode
	}
	return p.Default
}

// Decision describes whether a tool call is allowed and whether approval is
// required.
type Decision struct {
	Tool             string
	Mode             Mode
	Allowed          bool
	RequiresApproval bool
}

// Decide resolves the policy mode for tool and converts it into a Decision.
func (p *Policy) Decide(tool string) Decision {
	mode := p.Mode(tool)
	switch mode {
	case ModeAuto:
		return Decision{Tool: tool, Mode: mode, Allowed: true, RequiresApproval: false}
	case ModeConfirm:
		return Decision{Tool: tool, Mode: mode, Allowed: true, RequiresApproval: true}
	case ModeDeny:
		return Decision{Tool: tool, Mode: mode, Allowed: false, RequiresApproval: false}
	default:
		return Decision{Tool: tool, Mode: mode, Allowed: false, RequiresApproval: false}
	}
}

// AuditEntry records one policy decision for a tool.
type AuditEntry struct {
	Tool     string
	Decision Decision
	At       time.Time
}

// AuditLog is a concurrency-safe in-memory log of decisions.
type AuditLog struct {
	mu      sync.Mutex
	Entries []AuditEntry
}

// NewAuditLog returns an empty audit log.
func NewAuditLog() *AuditLog {
	return &AuditLog{}
}

// Record appends an audit entry with the current time.
func (l *AuditLog) Record(tool string, d Decision) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Entries = append(l.Entries, AuditEntry{
		Tool:     tool,
		Decision: d,
		At:       time.Now(),
	})
}

// List returns a copy of the recorded audit entries.
func (l *AuditLog) List() []AuditEntry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := make([]AuditEntry, len(l.Entries))
	copy(entries, l.Entries)
	return entries
}
