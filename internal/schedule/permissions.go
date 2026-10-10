package schedule

import "github.com/peggco/pegg/internal/core/config"

func isAskMode(mode string) bool {
	switch mode {
	case "", "ask", "semi-ask":
		return true
	default:
		return false
	}
}

func ApplyScheduledPermissions(p *config.PermissionConfig) {
	if p == nil {
		return
	}
	if isAskMode(p.Default) {
		p.Default = "allow"
	}
	for name, mode := range p.Rules {
		if isAskMode(mode) {
			p.Rules[name] = "allow"
		}
	}
}
