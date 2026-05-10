package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ChargeStatus string

const (
	StatusPending        ChargeStatus = "pending"
	StatusProcessing     ChargeStatus = "processing"
	StatusBoletoGenerated ChargeStatus = "boleto_generated"
	StatusFailed         ChargeStatus = "failed"
)

type Charge struct {
	ID             string
	ChargeID       string
	CustomerID     string
	CustomerName   string
	CustomerDoc    string
	AmountCents    int64
	DueDate        time.Time
	Description    string
	Status         ChargeStatus
	IdempotencyKey string
	Attempts       int
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ChargeRepository interface {
	Upsert(ctx context.Context, charge *Charge) error
	UpdateStatus(ctx context.Context, chargeID string, status ChargeStatus, lastError string) error
	GetByChargeID(ctx context.Context, chargeID string) (*Charge, error)
}

type PostgresChargeRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresChargeRepository(pool *pgxpool.Pool) *PostgresChargeRepository {
	return &PostgresChargeRepository{pool: pool}
}

func (r *PostgresChargeRepository) Upsert(ctx context.Context, charge *Charge) error {
	query := `
		INSERT INTO charges (
			charge_id, customer_id, customer_name, customer_doc,
			amount_cents, due_date, description, status, idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (idempotency_key) DO UPDATE SET
			status     = EXCLUDED.status,
			updated_at = NOW()`

	_, err := r.pool.Exec(ctx, query,
		charge.ChargeID,
		charge.CustomerID,
		charge.CustomerName,
		charge.CustomerDoc,
		charge.AmountCents,
		charge.DueDate,
		charge.Description,
		charge.Status,
		charge.IdempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("upsert charge %s: %w", charge.ChargeID, err)
	}
	return nil
}

func (r *PostgresChargeRepository) UpdateStatus(ctx context.Context, chargeID string, status ChargeStatus, lastError string) error {
	query := `
		UPDATE charges
		SET status     = $1,
		    last_error = NULLIF($2, ''),
		    attempts   = attempts + 1,
		    updated_at = NOW()
		WHERE charge_id = $3`

	_, err := r.pool.Exec(ctx, query, status, lastError, chargeID)
	if err != nil {
		return fmt.Errorf("update status charge %s: %w", chargeID, err)
	}
	return nil
}

func (r *PostgresChargeRepository) GetByChargeID(ctx context.Context, chargeID string) (*Charge, error) {
	query := `
		SELECT charge_id, customer_id, customer_name, customer_doc,
		       amount_cents, due_date, description, status,
		       idempotency_key, attempts, COALESCE(last_error, ''),
		       created_at, updated_at
		FROM charges
		WHERE charge_id = $1`

	row := r.pool.QueryRow(ctx, query, chargeID)

	var c Charge
	err := row.Scan(
		&c.ChargeID, &c.CustomerID, &c.CustomerName, &c.CustomerDoc,
		&c.AmountCents, &c.DueDate, &c.Description, &c.Status,
		&c.IdempotencyKey, &c.Attempts, &c.LastError,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get charge %s: %w", chargeID, err)
	}
	return &c, nil
}
