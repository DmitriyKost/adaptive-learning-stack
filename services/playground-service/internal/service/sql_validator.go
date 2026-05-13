package service

import (
	"fmt"
	"strings"
)

type QueryMode string

const (
	QueryModeUser      QueryMode = "user"
	QueryModeReference QueryMode = "reference"
)

func NormalizeAndValidateSQL(raw string, mode QueryMode) (string, error) {
	query := strings.TrimSpace(raw)
	if query == "" {
		return "", fmt.Errorf("empty sql")
	}
	if strings.ContainsRune(query, '\x00') {
		return "", fmt.Errorf("sql contains zero byte")
	}

	query = strings.TrimRight(query, " \t\n\r;")
	if query == "" {
		return "", fmt.Errorf("empty sql")
	}
	if strings.Contains(query, ";") {
		return "", fmt.Errorf("only one SQL statement is allowed")
	}

	upper := strings.ToUpper(query)
	collapsed := collapseWhitespace(upper)

	blocked := []string{
		"ALTER SYSTEM",
		"CREATE DATABASE",
		"DROP DATABASE",
		"CREATE EXTENSION",
		"DROP EXTENSION",
		"CREATE SERVER",
		"CREATE FOREIGN",
		"IMPORT FOREIGN SCHEMA",
		"CREATE FUNCTION",
		"CREATE PROCEDURE",
		"SECURITY DEFINER",
		"DROP SCHEMA",
		"ALTER SCHEMA",
		"GRANT ",
		"REVOKE ",
		"COPY ",
		"CALL ",
		"DO ",
		"SET ROLE",
		"RESET ROLE",
		"SET SESSION AUTHORIZATION",
		"RESET SESSION AUTHORIZATION",
		"LISTEN ",
		"NOTIFY ",
		"VACUUM",
		"ANALYZE",
		"LOAD ",
	}
	for _, token := range blocked {
		if strings.Contains(collapsed, token) {
			return "", fmt.Errorf("forbidden SQL operation: %s", strings.TrimSpace(token))
		}
	}

	if mode == QueryModeReference {
		if startsWithAny(collapsed, "SELECT", "WITH") {
			return query, nil
		}
		return "", fmt.Errorf("reference query may contain only SELECT or WITH")
	}

	if startsWithAny(collapsed,
		"SELECT",
		"WITH",
		"INSERT",
		"UPDATE",
		"DELETE",
		"CREATE TABLE",
		"DROP TABLE",
		"TRUNCATE TABLE",
	) {
		return query, nil
	}

	return "", fmt.Errorf("unsupported SQL operation")
}

func startsWithAny(value string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func collapseWhitespace(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}
