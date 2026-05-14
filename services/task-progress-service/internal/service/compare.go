package service

import (
	"fmt"
	"sort"
)

func compareTabularResults(actualColumns []string, actualRows [][]any, expectedColumns []string, expectedRows [][]any, orderSensitive bool) bool {
	if len(actualColumns) != len(expectedColumns) {
		return false
	}
	if len(actualRows) != len(expectedRows) {
		return false
	}
	for i := range actualColumns {
		if actualColumns[i] != expectedColumns[i] {
			return false
		}
	}

	actualNormalized := normalizeRows(actualRows)
	expectedNormalized := normalizeRows(expectedRows)

	if !orderSensitive {
		sort.Strings(actualNormalized)
		sort.Strings(expectedNormalized)
	}

	for i := range actualNormalized {
		if actualNormalized[i] != expectedNormalized[i] {
			return false
		}
	}
	return true
}

func normalizeRows(rows [][]any) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, fmt.Sprintf("%q", row))
	}
	return out
}
