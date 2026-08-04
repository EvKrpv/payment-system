package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrKeyInProgress         = errors.New("request with this idempotency key is already in progress")
	ErrKeyReusedWithDiffBody = errors.New("idempotency key reused with different request body")
)

type IdempotencyService struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewIdempotencyService(pool *pgxpool.Pool) *IdempotencyService {
	return &IdempotencyService{
		pool:   pool,
		logger: slog.Default(),
	}
}

func (s *IdempotencyService) SetLogger(logger *slog.Logger) {
	s.logger = logger
}

type IdempotencyResult struct {
	IsNew    bool
	Status   string
	Body     json.RawMessage
	HTTPCode int
}

// CheckOrCreate проверяет или создаёт запись идемпотентности
func (s *IdempotencyService) CheckOrCreate(ctx context.Context, key, ownerID string, body []byte) (*IdempotencyResult, error) {
	hash := hashRequest(body)
	s.logger.Info("checking idempotency", "key", key, "owner_id", ownerID, "hash", hash)

	// 1. Сначала проверяем, существует ли уже ключ
	var existingHash string
	var status string
	var httpCode int
	var responseBody []byte

	query := `
		SELECT request_hash, status, COALESCE(response_status_code, 0), COALESCE(response_body, '{}'::jsonb)
		FROM idempotency_keys
		WHERE idempotency_key = $1 AND owner_id = $2
	`

	err := s.pool.QueryRow(ctx, query, key, ownerID).Scan(
		&existingHash,
		&status,
		&httpCode,
		&responseBody,
	)

	if err == nil {
		// Ключ уже существует!
		s.logger.Info("key exists", "status", status, "hash_match", existingHash == hash)

		if existingHash != hash {
			return nil, ErrKeyReusedWithDiffBody
		}

		switch status {
		case "in_progress":
			return nil, ErrKeyInProgress
		case "completed":
			return &IdempotencyResult{
				IsNew:    false,
				Status:   status,
				Body:     responseBody,
				HTTPCode: httpCode,
			}, nil
		case "failed":
			// Если запрос упал, разрешаем повторить
			return &IdempotencyResult{IsNew: true}, nil
		default:
			return nil, fmt.Errorf("unknown status: %s", status)
		}
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing key: %w", err)
	}

	// 2. Ключ не найден — создаём новый
	insertQuery := `
		INSERT INTO idempotency_keys (idempotency_key, owner_id, request_hash, status)
		VALUES ($1, $2, $3, 'in_progress')
	`

	_, err = s.pool.Exec(ctx, insertQuery, key, ownerID, hash)
	if err != nil {
		// Если возникла ошибка дубликата (гонка), пробуем прочитать ещё раз
		if isDuplicateKeyError(err) {
			s.logger.Info("duplicate key after insert, retrying read")
			// Читаем существующую запись
			err = s.pool.QueryRow(ctx, query, key, ownerID).Scan(
				&existingHash,
				&status,
				&httpCode,
				&responseBody,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to get existing key after duplicate: %w", err)
			}

			if existingHash != hash {
				return nil, ErrKeyReusedWithDiffBody
			}

			switch status {
			case "in_progress":
				return nil, ErrKeyInProgress
			case "completed":
				return &IdempotencyResult{
					IsNew:    false,
					Status:   status,
					Body:     responseBody,
					HTTPCode: httpCode,
				}, nil
			case "failed":
				return &IdempotencyResult{IsNew: true}, nil
			default:
				return nil, fmt.Errorf("unknown status: %s", status)
			}
		}
		return nil, fmt.Errorf("failed to insert idempotency key: %w", err)
	}

	s.logger.Info("new idempotency record created")
	return &IdempotencyResult{IsNew: true}, nil
}

// Complete завершает запись идемпотентности
func (s *IdempotencyService) Complete(ctx context.Context, key, ownerID string, httpCode int, body []byte) error {
	s.logger.Info("completing idempotency", "key", key, "owner_id", ownerID)

	query := `
		UPDATE idempotency_keys
		SET status = 'completed',
		    response_status_code = $1,
		    response_body = $2,
		    updated_at = NOW()
		WHERE idempotency_key = $3 AND owner_id = $4
	`

	result, err := s.pool.Exec(ctx, query, httpCode, body, key, ownerID)
	if err != nil {
		return fmt.Errorf("failed to complete idempotency key: %w", err)
	}

	if result.RowsAffected() == 0 {
		s.logger.Warn("no rows updated, key not found", "key", key, "owner_id", ownerID)
		return fmt.Errorf("idempotency key not found: %s", key)
	}

	s.logger.Info("idempotency completed successfully", "key", key)
	return nil
}

// Fail помечает запись как failed
func (s *IdempotencyService) Fail(ctx context.Context, key, ownerID string) error {
	s.logger.Info("marking idempotency as failed", "key", key, "owner_id", ownerID)

	query := `
		UPDATE idempotency_keys
		SET status = 'failed', updated_at = NOW()
		WHERE idempotency_key = $1 AND owner_id = $2
	`

	result, err := s.pool.Exec(ctx, query, key, ownerID)
	if err != nil {
		return fmt.Errorf("failed to mark idempotency key as failed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("idempotency key not found: %s", key)
	}

	return nil
}

func hashRequest(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "ERROR: duplicate key value violates unique constraint \"idempotency_keys_owner_id_idempotency_key_key\" (SQLSTATE 23505)"
}
