package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/EvKrpv/payment-system/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrIdempotencyKeyExists = errors.New("idempotency key already exists")
	ErrKeyReused            = errors.New("idempotency key reused with different body")
)

// PaymentRepository — работа с платежами в БД
type PaymentRepository struct {
	pool *pgxpool.Pool
}

// NewPaymentRepository создаёт новый репозиторий
func NewPaymentRepository(pool *pgxpool.Pool) *PaymentRepository {
	return &PaymentRepository{pool: pool}
}

// CreatePayment создаёт новый платёж
func (r *PaymentRepository) CreatePayment(ctx context.Context, req models.CreatePaymentRequest) (*models.Payment, error) {
	paymentID := uuid.New()
	now := time.Now().UTC()

	query := `
		INSERT INTO payments (id, amount, currency, sender_id, receiver_id, description, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, amount, currency, sender_id, receiver_id, description, status, created_at, updated_at
	`

	var payment models.Payment
	err := r.pool.QueryRow(ctx, query,
		paymentID,
		req.Amount,
		req.Currency,
		req.SenderID,
		req.ReceiverID,
		req.Description,
		"pending",
		now,
		now,
	).Scan(
		&payment.ID,
		&payment.Amount,
		&payment.Currency,
		&payment.SenderID,
		&payment.ReceiverID,
		&payment.Description,
		&payment.Status,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	return &payment, nil
}

// GetPaymentByID возвращает платёж по ID
func (r *PaymentRepository) GetPaymentByID(ctx context.Context, id uuid.UUID) (*models.Payment, error) {
	query := `
		SELECT id, amount, currency, sender_id, receiver_id, description, status, created_at, updated_at
		FROM payments
		WHERE id = $1
	`

	var payment models.Payment
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&payment.ID,
		&payment.Amount,
		&payment.Currency,
		&payment.SenderID,
		&payment.ReceiverID,
		&payment.Description,
		&payment.Status,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	return &payment, nil
}

// UpdatePaymentStatus обновляет статус платежа
func (r *PaymentRepository) UpdatePaymentStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `
		UPDATE payments
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrPaymentNotFound
	}

	return nil
}
