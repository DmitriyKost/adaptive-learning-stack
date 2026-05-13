package service

import (
	"fmt"
	"math"
	"sort"
)

func compareTabularResults(actualColumns []string, actualRows [][]any, expectedColumns []string, expectedRows [][]any) bool {
	if len(actualColumns) != len(expectedColumns) || len(actualRows) != len(expectedRows) {
		return false
	}
	for i := range actualColumns {
		if actualColumns[i] != expectedColumns[i] {
			return false
		}
	}
	actualNormalized := normalizeRows(actualRows)
	expectedNormalized := normalizeRows(expectedRows)
	if len(actualNormalized) != len(expectedNormalized) {
		return false
	}
	sort.Strings(actualNormalized)
	sort.Strings(expectedNormalized)
	for i := range actualNormalized {
		if actualNormalized[i] != expectedNormalized[i] {
			return false
		}
	}
	return true
}

func normalizeRows(rows [][]any) []string {
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		items := make([]string, 0, len(row))
		for _, value := range row {
			items = append(items, normalizeValue(value))
		}
		result = append(result, fmt.Sprintf("%q", items))
	}
	return result
}

func normalizeValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case float64:
		if math.Abs(v-math.Round(v)) < 1e-9 {
			return fmt.Sprintf("%.0f", v)
		}
		return fmt.Sprintf("%.9f", v)
	case float32:
		f := float64(v)
		if math.Abs(f-math.Round(f)) < 1e-9 {
			return fmt.Sprintf("%.0f", f)
		}
		return fmt.Sprintf("%.9f", f)
	default:
		return fmt.Sprintf("%v", value)
	}
}
