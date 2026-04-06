package sqlgate

import (
	"regexp"
	"strings"
)

// SQLClassification holds the parsed metadata of a SQL query.
type SQLClassification struct {
	Operation      string // select, insert, update, delete, drop, truncate, create, alter, grant, revoke
	Table          string // extracted table name, if any
	HasWhereClause bool   // whether the query contains a WHERE clause
	IsDangerous    bool   // true for DROP, TRUNCATE, or DELETE/UPDATE without WHERE
}

var (
	reLeadingWhitespace = regexp.MustCompile(`^\s+`)
	reExtraSpaces       = regexp.MustCompile(`\s+`)
)

// ClassifySQL performs keyword-based classification of a SQL query string.
// It is intentionally simple and does not attempt full SQL parsing.
func ClassifySQL(query string) SQLClassification {
	// Normalize: trim, collapse whitespace, uppercase for matching.
	normalized := strings.TrimSpace(query)
	normalized = reExtraSpaces.ReplaceAllString(normalized, " ")
	upper := strings.ToUpper(normalized)

	c := SQLClassification{}
	c.HasWhereClause = strings.Contains(upper, " WHERE ")

	switch {
	case strings.HasPrefix(upper, "SELECT"):
		c.Operation = "select"
		c.Table = extractAfterKeyword(upper, "FROM")

	case strings.HasPrefix(upper, "INSERT INTO"):
		c.Operation = "insert"
		c.Table = extractTableToken(upper, "INSERT INTO")

	case strings.HasPrefix(upper, "INSERT"):
		c.Operation = "insert"
		c.Table = extractTableToken(upper, "INSERT")

	case strings.HasPrefix(upper, "UPDATE"):
		c.Operation = "update"
		c.Table = extractTableToken(upper, "UPDATE")
		if !c.HasWhereClause {
			c.IsDangerous = true
		}

	case strings.HasPrefix(upper, "DELETE FROM"):
		c.Operation = "delete"
		c.Table = extractTableToken(upper, "DELETE FROM")
		if !c.HasWhereClause {
			c.IsDangerous = true
		}

	case strings.HasPrefix(upper, "DELETE"):
		c.Operation = "delete"
		c.Table = extractAfterKeyword(upper, "FROM")
		if !c.HasWhereClause {
			c.IsDangerous = true
		}

	case strings.HasPrefix(upper, "DROP DATABASE"):
		c.Operation = "drop_database"
		c.Table = extractTableToken(upper, "DROP DATABASE")
		c.IsDangerous = true

	case strings.HasPrefix(upper, "DROP TABLE"):
		c.Operation = "drop_table"
		c.Table = extractTableToken(upper, "DROP TABLE")
		c.IsDangerous = true

	case strings.HasPrefix(upper, "TRUNCATE TABLE"):
		c.Operation = "truncate"
		c.Table = extractTableToken(upper, "TRUNCATE TABLE")
		c.IsDangerous = true

	case strings.HasPrefix(upper, "TRUNCATE"):
		c.Operation = "truncate"
		c.Table = extractTableToken(upper, "TRUNCATE")
		c.IsDangerous = true

	case strings.HasPrefix(upper, "CREATE TABLE"):
		c.Operation = "create_table"
		c.Table = extractTableToken(upper, "CREATE TABLE")

	case strings.HasPrefix(upper, "ALTER TABLE"):
		c.Operation = "alter_table"
		c.Table = extractTableToken(upper, "ALTER TABLE")

	case strings.HasPrefix(upper, "GRANT"):
		c.Operation = "grant"

	case strings.HasPrefix(upper, "REVOKE"):
		c.Operation = "revoke"

	default:
		c.Operation = "unknown"
	}

	// Lowercase the table name for consistency.
	c.Table = strings.ToLower(c.Table)

	return c
}

// extractTableToken returns the first token after the given prefix keyword(s).
func extractTableToken(upper, prefix string) string {
	rest := strings.TrimPrefix(upper, prefix)
	rest = strings.TrimSpace(rest)
	// Handle IF EXISTS / IF NOT EXISTS
	if strings.HasPrefix(rest, "IF EXISTS ") {
		rest = strings.TrimPrefix(rest, "IF EXISTS ")
		rest = strings.TrimSpace(rest)
	}
	if strings.HasPrefix(rest, "IF NOT EXISTS ") {
		rest = strings.TrimPrefix(rest, "IF NOT EXISTS ")
		rest = strings.TrimSpace(rest)
	}
	parts := strings.Fields(rest)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// extractAfterKeyword finds the keyword in the string and returns the next token.
func extractAfterKeyword(upper, keyword string) string {
	idx := strings.Index(upper, " "+keyword+" ")
	if idx < 0 {
		return ""
	}
	rest := upper[idx+len(keyword)+2:]
	rest = strings.TrimSpace(rest)
	parts := strings.Fields(rest)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
