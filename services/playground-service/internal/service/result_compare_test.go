package service

import (
	"testing"

	"playground-service/internal/domain"
)

func TestCompareExecuteResultsOrderInsensitive(t *testing.T) {
	expected := &domain.ExecuteResult{
		Columns: []string{"id", "name"},
		Rows: []map[string]any{
			{"id": 1, "name": "Ivan"},
			{"id": 2, "name": "Anna"},
		},
	}
	actual := &domain.ExecuteResult{
		Columns: []string{"id", "name"},
		Rows: []map[string]any{
			{"id": 2, "name": "Anna"},
			{"id": 1, "name": "Ivan"},
		},
	}

	if !compareExecuteResults(actual, expected, false) {
		t.Fatal("expected different row order to match when orderSensitive=false")
	}
}

func TestCompareExecuteResultsOrderSensitive(t *testing.T) {
	expected := &domain.ExecuteResult{
		Columns: []string{"id", "name"},
		Rows: []map[string]any{
			{"id": 1, "name": "Ivan"},
			{"id": 2, "name": "Anna"},
		},
	}
	actualDifferentOrder := &domain.ExecuteResult{
		Columns: []string{"id", "name"},
		Rows: []map[string]any{
			{"id": 2, "name": "Anna"},
			{"id": 1, "name": "Ivan"},
		},
	}

	if compareExecuteResults(actualDifferentOrder, expected, true) {
		t.Fatal("expected different row order not to match when orderSensitive=true")
	}
}
