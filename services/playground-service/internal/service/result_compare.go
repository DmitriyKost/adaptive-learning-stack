package service

import (
	"fmt"
	"sort"

	"playground-service/internal/domain"
)

func compareExecuteResults(actual, expected *domain.ExecuteResult, orderSensitive bool) bool {
	if actual == nil || expected == nil {
		return false
	}
	if actual.Error != nil || expected.Error != nil {
		return false
	}
	if len(actual.Columns) != len(expected.Columns) {
		return false
	}
	if len(actual.Rows) != len(expected.Rows) {
		return false
	}
	for i := range actual.Columns {
		if actual.Columns[i] != expected.Columns[i] {
			return false
		}
	}

	actualRows := normalizeExecuteRows(actual.Columns, actual.Rows)
	expectedRows := normalizeExecuteRows(expected.Columns, expected.Rows)

	if !orderSensitive {
		sort.Strings(actualRows)
		sort.Strings(expectedRows)
	}

	for i := range actualRows {
		if actualRows[i] != expectedRows[i] {
			return false
		}
	}

	return true
}

func normalizeExecuteRows(columns []string, rows []map[string]any) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, normalizeExecuteValue(row[column]))
		}
		out = append(out, fmt.Sprintf("%q", values))
	}
	return out
}

func normalizeExecuteValue(value any) string {
	if value == nil {
		return "NULL"
	}
	return fmt.Sprintf("%v", value)
}
