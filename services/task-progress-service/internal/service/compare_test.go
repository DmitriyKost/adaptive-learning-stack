package service

import "testing"

func TestCompareTabularResultsOrderInsensitive(t *testing.T) {
	expectedColumns := []string{"id", "name"}
	expectedRows := [][]any{
		{1, "Ivan"},
		{2, "Anna"},
	}
	actualColumns := []string{"id", "name"}
	actualRows := [][]any{
		{2, "Anna"},
		{1, "Ivan"},
	}

	if !compareTabularResults(actualColumns, actualRows, expectedColumns, expectedRows, false) {
		t.Fatal("expected different row order to match when orderSensitive=false")
	}
}

func TestCompareTabularResultsOrderSensitive(t *testing.T) {
	expectedColumns := []string{"id", "name"}
	expectedRows := [][]any{
		{1, "Ivan"},
		{2, "Anna"},
	}
	actualColumns := []string{"id", "name"}
	actualRows := [][]any{
		{2, "Anna"},
		{1, "Ivan"},
	}

	if compareTabularResults(actualColumns, actualRows, expectedColumns, expectedRows, true) {
		t.Fatal("expected different row order not to match when orderSensitive=true")
	}
}
