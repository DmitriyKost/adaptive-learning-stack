package service

import (
	"context"
	"fmt"
	"time"

	"playground-service/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExecutorOptions struct {
	ExecutionTimeout time.Duration
	StatementTimeout time.Duration
	LockTimeout      time.Duration
	MaxResultRows    int
	TaskSchema       string
	ReadonlyRole     string
}

type Executor struct {
	db      *pgxpool.Pool
	options ExecutorOptions
}

func NewExecutor(db *pgxpool.Pool, options ExecutorOptions) *Executor {
	return &Executor{db: db, options: options}
}

func (e *Executor) ExecuteUser(ctx context.Context, workspace domain.Workspace, rawSQL string) (*domain.ExecuteResult, error) {
	query, err := NormalizeAndValidateSQL(rawSQL, QueryModeUser)
	if err != nil {
		return &domain.ExecuteResult{Error: &domain.QueryError{Type: "validation_error", Message: err.Error()}}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, e.options.ExecutionTimeout)
	defer cancel()

	startedAt := time.Now()
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin user transaction: %w", err)
	}
	defer rollbackQuietly(ctx, tx)

	if err := e.prepareUserTransaction(ctx, tx, workspace); err != nil {
		return nil, err
	}

	result, queryErr := e.query(ctx, tx, query, startedAt)
	if queryErr != nil {
		result.QueryTimeMs = time.Since(startedAt).Milliseconds()
		result.Error = &domain.QueryError{Type: "query_error", Message: queryErr.Error()}
		return result, nil
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit user transaction: %w", err)
	}

	return result, nil
}

func (e *Executor) ExecuteReference(ctx context.Context, rawSQL string) (*domain.ExecuteResult, error) {
	query, err := NormalizeAndValidateSQL(rawSQL, QueryModeReference)
	if err != nil {
		return nil, fmt.Errorf("validate reference SQL: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, e.options.ExecutionTimeout)
	defer cancel()

	startedAt := time.Now()
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin reference transaction: %w", err)
	}
	defer rollbackQuietly(ctx, tx)

	if err := e.prepareReferenceTransaction(ctx, tx); err != nil {
		return nil, err
	}

	result, queryErr := e.query(ctx, tx, query, startedAt)
	if queryErr != nil {
		result.QueryTimeMs = time.Since(startedAt).Milliseconds()
		result.Error = &domain.QueryError{Type: "query_error", Message: queryErr.Error()}
		return result, nil
	}

	// Read-only transaction does not need to persist anything, but Commit releases it cleanly.
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reference transaction: %w", err)
	}

	return result, nil
}

func (e *Executor) prepareUserTransaction(ctx context.Context, tx pgx.Tx, workspace domain.Workspace) error {
	if err := setLocalRole(ctx, tx, workspace.RoleName); err != nil {
		return fmt.Errorf("set user role: %w", err)
	}
	if err := setTimeouts(ctx, tx, e.options.StatementTimeout, e.options.LockTimeout); err != nil {
		return err
	}

	searchPath := fmt.Sprintf("%s, %s, public",
		pgx.Identifier{workspace.SchemaName}.Sanitize(),
		pgx.Identifier{e.options.TaskSchema}.Sanitize(),
	)
	if _, err := tx.Exec(ctx, "SELECT set_config('search_path', $1, true)", searchPath); err != nil {
		return fmt.Errorf("set user search_path: %w", err)
	}

	return nil
}

func (e *Executor) prepareReferenceTransaction(ctx context.Context, tx pgx.Tx) error {
	if e.options.ReadonlyRole != "" {
		if err := setLocalRole(ctx, tx, e.options.ReadonlyRole); err != nil {
			return fmt.Errorf("set readonly role: %w", err)
		}
	}
	if err := setTimeouts(ctx, tx, e.options.StatementTimeout, e.options.LockTimeout); err != nil {
		return err
	}

	searchPath := fmt.Sprintf("%s, public", pgx.Identifier{e.options.TaskSchema}.Sanitize())
	if _, err := tx.Exec(ctx, "SELECT set_config('search_path', $1, true)", searchPath); err != nil {
		return fmt.Errorf("set reference search_path: %w", err)
	}

	return nil
}

func (e *Executor) query(ctx context.Context, tx pgx.Tx, query string, startedAt time.Time) (*domain.ExecuteResult, error) {
	rows, err := tx.Query(ctx, query)
	result := &domain.ExecuteResult{}
	if err != nil {
		return result, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	columns := make([]string, 0, len(fields))
	for _, field := range fields {
		columns = append(columns, field.Name)
	}
	result.Columns = columns

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return result, err
		}

		if len(result.Rows) >= e.options.MaxResultRows {
			result.Truncated = true
			continue
		}

		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = normalizeValue(values[i])
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	rows.Close()
	result.RowsAffected = rows.CommandTag().RowsAffected()
	result.QueryTimeMs = time.Since(startedAt).Milliseconds()
	return result, nil
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(v)
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	default:
		return v
	}
}

func setLocalRole(ctx context.Context, tx pgx.Tx, roleName string) error {
	_, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL ROLE %s", pgx.Identifier{roleName}.Sanitize()))
	return err
}

func setTimeouts(ctx context.Context, tx pgx.Tx, statementTimeout, lockTimeout time.Duration) error {
	if _, err := tx.Exec(ctx, "SELECT set_config('statement_timeout', $1, true)", durationAsPostgresMS(statementTimeout)); err != nil {
		return fmt.Errorf("set statement_timeout: %w", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('lock_timeout', $1, true)", durationAsPostgresMS(lockTimeout)); err != nil {
		return fmt.Errorf("set lock_timeout: %w", err)
	}
	return nil
}

func durationAsPostgresMS(d time.Duration) string {
	return fmt.Sprintf("%dms", d.Milliseconds())
}

func rollbackQuietly(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
