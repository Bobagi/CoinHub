package repository

import (
	"context"
	"time"
)

// HasSuccessfulExecutionOfTypeSince reports whether the user already has a successful execution of
// the given type for a specific pair since a timestamp. Scoping by pair keeps the daily purchase
// idempotent per robot (one buy per coin per day), so several robots do not block one another.
func (repository *PostgresTradingOperationExecutionRepository) HasSuccessfulExecutionOfTypeSince(loadContext context.Context, userIdentifier int64, environment string, operationType string, tradingPairSymbol string, since time.Time) (bool, error) {
	return repository.hasExecutionOfTypeSince(loadContext, userIdentifier, environment, operationType, tradingPairSymbol, since, true)
}

// HasFailedExecutionOfTypeSince is the failure-side twin, used to record at most ONE failed daily-buy
// history row per robot per day: the worker keeps retrying every tick within the hour (so transient
// errors still recover), but the history is not spammed with one row per attempt.
func (repository *PostgresTradingOperationExecutionRepository) HasFailedExecutionOfTypeSince(loadContext context.Context, userIdentifier int64, environment string, operationType string, tradingPairSymbol string, since time.Time) (bool, error) {
	return repository.hasExecutionOfTypeSince(loadContext, userIdentifier, environment, operationType, tradingPairSymbol, since, false)
}

func (repository *PostgresTradingOperationExecutionRepository) hasExecutionOfTypeSince(loadContext context.Context, userIdentifier int64, environment string, operationType string, tradingPairSymbol string, since time.Time, success bool) (bool, error) {
	row := repository.Database.QueryRowContext(
		loadContext,
		`SELECT EXISTS(
		    SELECT 1 FROM trading_operation_executions
		    WHERE user_id = $1 AND binance_environment = $2 AND operation_type = $3 AND trading_pair_symbol = $4 AND success = $6 AND executed_at >= $5
		 )`,
		userIdentifier, environment, operationType, tradingPairSymbol, since, success,
	)
	var executionExists bool
	if scanError := row.Scan(&executionExists); scanError != nil {
		return false, scanError
	}
	return executionExists, nil
}
