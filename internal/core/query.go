package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// A deliberately small grammar, not a regex search for a FROM substring.
// This prevents a string literal or alias from bypassing the table allowlist.
const ident = `[A-Za-z_][A-Za-z0-9_]*`

var selectHead = regexp.MustCompile(`(?is)^SELECT\s+(\*|` + ident + `(?:\s*,\s*` + ident + `)*)\s+FROM\s+(` + ident + `)\.(` + ident + `)(.*)$`)
var tailGrammar = regexp.MustCompile(`(?is)^\s*(?:WHERE\s+` + ident + `\s*(?:=|!=|<=|>=|<|>)\s*(?:'([^']|'')*'|[-+]?[0-9]+(?:\.[0-9]+)?|[a-f0-9]{8}-[a-f0-9-]{27,}|true|false)(?:\s+AND\s+` + ident + `\s*(?:=|!=|<=|>=|<|>)\s*(?:'([^']|'')*'|[-+]?[0-9]+(?:\.[0-9]+)?|[a-f0-9]{8}-[a-f0-9-]{27,}|true|false))*\s*)?(?:ORDER\s+BY\s+` + ident + `(?:\s+(?:ASC|DESC))?(?:\s*,\s*` + ident + `(?:\s+(?:ASC|DESC))?)*\s*)?(?:LIMIT\s+[0-9]+\s*)?$`)
var endLimit = regexp.MustCompile(`(?i)\s+LIMIT\s+([0-9]+)\s*$`)

func ValidateQuery(q string, allowUserTables bool) (string, error) {
	q = strings.TrimSpace(q)
	if len(q) == 0 || len(q) > 16000 {
		return "", fmt.Errorf("query must be 1..16000 bytes")
	}
	q = strings.TrimSpace(strings.TrimSuffix(q, ";"))
	if strings.ContainsAny(q, ";\x00") || strings.Contains(q, "--") || strings.Contains(q, "/*") || strings.Contains(q, "//") {
		return "", fmt.Errorf("comments and multiple statements are disabled")
	}
	m := selectHead.FindStringSubmatch(q)
	if len(m) != 5 {
		return "", fmt.Errorf("use SELECT columns FROM keyspace.table; expressions, aliases, functions and quoted identifiers are disabled")
	}
	allowed := map[string]bool{"system.cluster_status": true, "system.local": true, "system.peers": true, "system.peers_v2": true, "system.raft_state": true, "system.scylla_local": true, "system_schema.keyspaces": true, "system_schema.tables": true, "system_schema.columns": true}
	table := strings.ToLower(m[2] + "." + m[3])
	if !allowUserTables && !allowed[table] {
		return "", fmt.Errorf("this console only permits selected system tables")
	}
	if !tailGrammar.MatchString(m[4]) {
		return "", fmt.Errorf("only simple WHERE comparisons (AND), ORDER BY and LIMIT are supported; ALLOW FILTERING is disabled")
	}
	if l := endLimit.FindStringSubmatch(q); len(l) > 0 {
		n, e := strconv.Atoi(l[1])
		if e != nil || n < 1 {
			return "", fmt.Errorf("invalid LIMIT")
		}
	}
	return q, nil
}

// BoundQuery applies a database-side LIMIT as well as the client's row cap.
func BoundQuery(q string, limit int) string {
	cap := max(1, min(limit, 500)) + 1
	if l := endLimit.FindStringSubmatch(q); len(l) > 0 {
		n, e := strconv.Atoi(l[1])
		if e == nil && n <= cap {
			return q
		}
		return endLimit.ReplaceAllString(q, fmt.Sprintf(" LIMIT %d", cap))
	}
	return q + fmt.Sprintf(" LIMIT %d", cap)
}
