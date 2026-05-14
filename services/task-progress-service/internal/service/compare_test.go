package service

import "testing"

func TestCompareTabularResultsOrderInsensitiveWithoutOrderBy(t *testing.T) {
	expectedColumns := []string{"id", "name"}
	expectedRows := [][]any{
		{1, "Ivan"},
		{2, "Anna"},
		{3, "Petr"},
	}

	actualColumns := []string{"id", "name"}
	actualRowsDifferentOrder := [][]any{
		{3, "Petr"},
		{1, "Ivan"},
		{2, "Anna"},
	}

	if !compareTabularResults(actualColumns, actualRowsDifferentOrder, expectedColumns, expectedRows, false) {
		t.Fatal("expected rows with different order to match when orderSensitive=false")
	}
}

func TestCompareTabularResultsOrderSensitiveWithOrderBy(t *testing.T) {
	expectedColumns := []string{"id", "name"}
	expectedRows := [][]any{
		{1, "Ivan"},
		{2, "Anna"},
		{3, "Petr"},
	}

	actualColumns := []string{"id", "name"}
	actualRowsDifferentOrder := [][]any{
		{3, "Petr"},
		{1, "Ivan"},
		{2, "Anna"},
	}

	if compareTabularResults(actualColumns, actualRowsDifferentOrder, expectedColumns, expectedRows, true) {
		t.Fatal("expected rows with different order not to match when orderSensitive=true")
	}

	actualRowsSameOrder := [][]any{
		{1, "Ivan"},
		{2, "Anna"},
		{3, "Petr"},
	}

	if !compareTabularResults(actualColumns, actualRowsSameOrder, expectedColumns, expectedRows, true) {
		t.Fatal("expected rows with same order to match when orderSensitive=true")
	}
}

func TestCompareTabularResultsColumnOrderIsAlwaysSensitive(t *testing.T) {
	expectedColumns := []string{"id", "name"}
	expectedRows := [][]any{
		{1, "Ivan"},
	}

	actualColumns := []string{"name", "id"}
	actualRows := [][]any{
		{"Ivan", 1},
	}

	if compareTabularResults(actualColumns, actualRows, expectedColumns, expectedRows, false) {
		t.Fatal("expected different column order not to match")
	}
}

func TestTaskProgressIsOrderSensitiveReference(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want bool
	}{
		{
			name: "plain select",
			sql:  "SELECT id, name FROM task_data.employees;",
			want: false,
		},
		{
			name: "simple order by",
			sql:  "SELECT id, name FROM task_data.employees ORDER BY id;",
			want: true,
		},
		{
			name: "case insensitive",
			sql:  "select id, name from task_data.employees order by id;",
			want: true,
		},
		{
			name: "multiline order by",
			sql:  "SELECT id, name\nFROM task_data.employees\nORDER\nBY id;",
			want: true,
		},
		{
			name: "not order_by identifier",
			sql:  "SELECT order_by_column FROM example;",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOrderSensitiveReference(tc.sql); got != tc.want {
				t.Fatalf("isOrderSensitiveReference() = %v, want %v", got, tc.want)
			}
		})
	}
}
