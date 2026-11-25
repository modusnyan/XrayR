package api

import (
	"regexp"

	log "github.com/sirupsen/logrus"
)

// AppendRule compiles an audit-rule pattern and appends it to rules.
//
// Rule patterns arrive from a remote panel and are therefore untrusted. A
// malformed pattern must never crash the node, so compilation failures are logged
// and the rule is skipped instead of panicking.
func AppendRule(rules []DetectRule, id int, pattern string) []DetectRule {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		log.WithFields(log.Fields{"rule_id": id, "pattern": pattern}).WithError(err).
			Warn("Ignoring invalid audit rule pattern received from panel")
		return rules
	}
	return append(rules, DetectRule{ID: id, Pattern: compiled})
}
