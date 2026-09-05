package query

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"data-vision/backend/internal/datasource"
	"data-vision/backend/internal/model"
)

func testExecutor(t *testing.T) (*Executor, *datasource.Manager) {
	t.Helper()
	if os.Getenv("CGO_ENABLED") == "0" {
		t.Skip("SQLite integration tests require CGO_ENABLED=1; Docker enables CGO for go-sqlite3")
	}
	manager := datasource.NewManager("test-key")
	source := model.DataSource{UID: "test-sqlite", Name: "Test SQLite", Type: datasource.TypeSQLite, ConfigJSON: `{"path":"file:query-test?mode=memory&cache=shared"}`}
	if err := manager.Register(context.Background(), source); err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("SQLite integration tests require CGO_ENABLED=1; Docker enables CGO for go-sqlite3")
		}
		t.Fatal(err)
	}
	db, _, err := manager.DB(context.Background(), source.UID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE orders (id INTEGER PRIMARY KEY, region TEXT, amount INTEGER); INSERT INTO orders (region, amount) VALUES ('east', 10), ('west', 20), ('east', 5);`); err != nil {
		t.Fatal(err)
	}
	return NewExecutor(manager, 5*time.Second, 100, 1024*1024), manager
}

func TestExecuteSQLBindsNamedParamsAndNormalizesRows(t *testing.T) {
	executor, manager := testExecutor(t)
	defer manager.Close()
	result, err := executor.Execute(context.Background(), Config{Mode: "sql", DatasourceUID: "test-sqlite", SQL: "SELECT region, amount FROM orders WHERE amount >= :minimum ORDER BY amount", Params: map[string]any{"minimum": 10}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 || result.Rows[0]["amount"] != float64(10) {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Columns[1].Type != "number" {
		t.Fatalf("expected number column, got %#v", result.Columns)
	}
}

func TestExecuteRejectsWritesAndSupportsPipeline(t *testing.T) {
	executor, manager := testExecutor(t)
	defer manager.Close()
	if _, err := executor.Execute(context.Background(), Config{Mode: "sql", DatasourceUID: "test-sqlite", SQL: "DELETE FROM orders"}); err == nil {
		t.Fatal("expected write query to be rejected")
	}
	queryA, _ := json.Marshal(Config{Mode: "sql", DatasourceUID: "test-sqlite", SQL: "SELECT region, amount FROM orders"})
	queryB, _ := json.Marshal(Config{Mode: "sql", DatasourceUID: "test-sqlite", SQL: "SELECT region, 'ok' AS status FROM orders GROUP BY region"})
	config := Config{Mode: "pipeline", Nodes: []PipelineNode{
		{ID: "amounts", Kind: "source", Query: queryA, Alias: "amounts"},
		{ID: "regions", Kind: "source", Query: queryB, Alias: "regions"},
		{ID: "joined", Kind: "join", Left: "amounts", Right: "regions", JoinType: "left", LeftKeys: []string{"region"}, RightKeys: []string{"region"}},
		{ID: "calculated", Kind: "calculate", Input: "joined", Fields: map[string]string{"double_amount": "amount * 2"}},
		{ID: "summary", Kind: "aggregate", Input: "calculated", GroupBy: []string{"region"}, Aggregates: map[string]Aggregate{"total": {Op: "sum", Field: "double_amount"}}},
	}, OutputNodeID: "summary"}
	result, err := executor.Execute(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected two regions, got %#v", result.Rows)
	}
}

func TestHTTPRowsExtractsNestedArray(t *testing.T) {
	rows, err := httpRows(map[string]any{"data": map[string]any{"items": []any{map[string]any{"name": "east", "value": 2}}}}, "data.items", map[string]string{"label": "name", "amount": "value"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["label"] != "east" {
		t.Fatalf("unexpected HTTP rows: %#v", rows)
	}
}

func TestValidateSQLAllowsReadOnlyQueriesOnly(t *testing.T) {
	for _, value := range []string{"SELECT 1", "WITH rows AS (SELECT 1) SELECT * FROM rows"} {
		if err := validateSQL(value); err != nil {
			t.Errorf("expected %q to pass: %v", value, err)
		}
	}
	for _, value := range []string{"UPDATE orders SET amount = 1", "SELECT 1; SELECT 2", "WITH removed AS (DELETE FROM orders) SELECT * FROM removed"} {
		if err := validateSQL(value); err == nil {
			t.Errorf("expected %q to be rejected", value)
		}
	}
}
