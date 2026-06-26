package repository

import "context"

// ListActiveUserIdentifiers returns the ids of all active users, for background automation to iterate.
func (repository *PostgresUserRepository) ListActiveUserIdentifiers(loadContext context.Context) ([]int64, error) {
	rows, queryError := repository.Database.QueryContext(loadContext, "SELECT id FROM users WHERE is_active = true ORDER BY id")
	if queryError != nil {
		return nil, queryError
	}
	defer rows.Close()

	userIdentifiers := make([]int64, 0)
	for rows.Next() {
		var userIdentifier int64
		if scanError := rows.Scan(&userIdentifier); scanError != nil {
			return nil, scanError
		}
		userIdentifiers = append(userIdentifiers, userIdentifier)
	}
	return userIdentifiers, rows.Err()
}

// ListActiveUserIdentifiersForShard returns the ids of active users that belong to one worker shard:
// those where id % shardCount == shardIndex. This lets several worker instances run in parallel, each
// processing a disjoint slice of users (deterministic, no coordination needed). shardCount <= 1 returns
// every active user (the single-worker default), so behaviour is unchanged unless sharding is enabled.
func (repository *PostgresUserRepository) ListActiveUserIdentifiersForShard(loadContext context.Context, shardCount int, shardIndex int) ([]int64, error) {
	if shardCount <= 1 {
		return repository.ListActiveUserIdentifiers(loadContext)
	}
	rows, queryError := repository.Database.QueryContext(
		loadContext,
		"SELECT id FROM users WHERE is_active = true AND (id % $1) = $2 ORDER BY id",
		shardCount, shardIndex,
	)
	if queryError != nil {
		return nil, queryError
	}
	defer rows.Close()

	userIdentifiers := make([]int64, 0)
	for rows.Next() {
		var userIdentifier int64
		if scanError := rows.Scan(&userIdentifier); scanError != nil {
			return nil, scanError
		}
		userIdentifiers = append(userIdentifiers, userIdentifier)
	}
	return userIdentifiers, rows.Err()
}

// ListAdminEmails returns the email addresses of active admin users, used to page operators about
// infrastructure problems (e.g. a stalled automation worker).
func (repository *PostgresUserRepository) ListAdminEmails(loadContext context.Context) ([]string, error) {
	rows, queryError := repository.Database.QueryContext(loadContext, "SELECT email FROM users WHERE is_active = true AND is_admin = true ORDER BY id")
	if queryError != nil {
		return nil, queryError
	}
	defer rows.Close()

	adminEmails := make([]string, 0)
	for rows.Next() {
		var adminEmail string
		if scanError := rows.Scan(&adminEmail); scanError != nil {
			return nil, scanError
		}
		adminEmails = append(adminEmails, adminEmail)
	}
	return adminEmails, rows.Err()
}
