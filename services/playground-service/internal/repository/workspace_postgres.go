package repository

import (
	"context"
	"fmt"

	"playground-service/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkspaceRepository struct {
	db             *pgxpool.Pool
	internalSchema string
}

func NewWorkspaceRepository(db *pgxpool.Pool, internalSchema string) *WorkspaceRepository {
	return &WorkspaceRepository{db: db, internalSchema: internalSchema}
}

func (r *WorkspaceRepository) EnsureWorkspace(ctx context.Context, userID string) (domain.Workspace, error) {
	query := fmt.Sprintf(
		"SELECT schema_name, role_name FROM %s.ensure_user_workspace($1::uuid)",
		pgIdent(r.internalSchema),
	)

	workspace := domain.Workspace{UserID: userID}
	if err := r.db.QueryRow(ctx, query, userID).Scan(&workspace.SchemaName, &workspace.RoleName); err != nil {
		return domain.Workspace{}, fmt.Errorf("ensure user workspace: %w", err)
	}

	return workspace, nil
}

func (r *WorkspaceRepository) ResetWorkspace(ctx context.Context, userID string) (domain.Workspace, error) {
	query := fmt.Sprintf(
		"SELECT schema_name, role_name FROM %s.reset_user_workspace($1::uuid)",
		pgIdent(r.internalSchema),
	)

	workspace := domain.Workspace{UserID: userID}
	if err := r.db.QueryRow(ctx, query, userID).Scan(&workspace.SchemaName, &workspace.RoleName); err != nil {
		return domain.Workspace{}, fmt.Errorf("reset user workspace: %w", err)
	}

	return workspace, nil
}

func pgIdent(value string) string {
	result := `"`
	for _, r := range value {
		if r == '"' {
			result += `""`
			continue
		}
		result += string(r)
	}
	result += `"`
	return result
}
