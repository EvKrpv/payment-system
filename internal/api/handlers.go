package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/EvKrpv/payment-system/internal/idempotency"
	"github.com/EvKrpv/payment-system/internal/models"
	"github.com/EvKrpv/payment-system/internal/repository"
	"github.com/google/uuid"
)

// PaymentHandlers — обработчики для платежей
type PaymentHandlers struct {
	paymentRepo    *repository.PaymentRepository
	idempotencySvc *idempotency.IdempotencyService
	logger         *slog.Logger
}

// NewPaymentHandlers создаёт новые хендлеры
func NewPaymentHandlers(
	paymentRepo *repository.PaymentRepository,
	idempotencySvc *idempotency.IdempotencyService,
	logger *slog.Logger,
) *PaymentHandlers {
	return &PaymentHandlers{
		paymentRepo:    paymentRepo,
		idempotencySvc: idempotencySvc,
		logger:         logger,
	}
}

// CreatePayment — POST /api/v1/payments
func (h *PaymentHandlers) CreatePayment(w http.ResponseWriter, r *http.Request) {
	// 1. Проверяем метод
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// 2. Читаем тело запроса
	var req models.CreatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 3. Валидируем запрос
	if err := validatePaymentRequest(req); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// 4. Получаем idempotency key из заголовка
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		http.Error(w, `{"error":"Idempotency-Key header is required"}`, http.StatusBadRequest)
		return
	}

	// 5. Проверяем идемпотентность
	ownerID := req.SenderID // используем sender_id как owner_id
	bodyBytes, _ := json.Marshal(req)

	result, err := h.idempotencySvc.CheckOrCreate(r.Context(), idempotencyKey, ownerID, bodyBytes)
	if err != nil {
		if err == idempotency.ErrKeyReusedWithDiffBody {
			http.Error(w, `{"error":"idempotency key reused with different request body"}`, http.StatusUnprocessableEntity)
			return
		}
		if err == idempotency.ErrKeyInProgress {
			http.Error(w, `{"error":"request with this idempotency key is already in progress"}`, http.StatusConflict)
			return
		}
		h.logger.Error("idempotency check failed", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// 6. Если это повторный запрос — возвращаем сохранённый ответ
	if !result.IsNew {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(result.HTTPCode)
		w.Write(result.Body)
		return
	}

	// 7. Создаём платёж
	payment, err := h.paymentRepo.CreatePayment(r.Context(), req)
	if err != nil {
		// Помечаем ключ как failed
		h.idempotencySvc.Fail(r.Context(), idempotencyKey, ownerID)
		h.logger.Error("failed to create payment", "error", err)
		http.Error(w, `{"error":"failed to create payment"}`, http.StatusInternalServerError)
		return
	}

	// 8. Формируем ответ
	response := models.PaymentResponse{
		PaymentID:   payment.ID.String(),
		Amount:      payment.Amount,
		Currency:    payment.Currency,
		SenderID:    payment.SenderID,
		ReceiverID:  payment.ReceiverID,
		Description: payment.Description,
		Status:      payment.Status,
		CreatedAt:   payment.CreatedAt.Format(time.RFC3339),
	}

	responseBytes, _ := json.Marshal(response)

	// 9. Сохраняем результат идемпотентности
	if err := h.idempotencySvc.Complete(r.Context(), idempotencyKey, ownerID, http.StatusCreated, responseBytes); err != nil {
		h.logger.Error("failed to complete idempotency", "error", err)
	}

	// 10. Возвращаем ответ
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(responseBytes)
}

// GetPayment — GET /api/v1/payments/{id}
func (h *PaymentHandlers) GetPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Извлекаем ID из URL
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/payments/")
	if path == "" {
		http.Error(w, `{"error":"payment id is required"}`, http.StatusBadRequest)
		return
	}

	paymentID, err := uuid.Parse(path)
	if err != nil {
		http.Error(w, `{"error":"invalid payment id"}`, http.StatusBadRequest)
		return
	}

	payment, err := h.paymentRepo.GetPaymentByID(r.Context(), paymentID)
	if err != nil {
		if err == repository.ErrPaymentNotFound {
			http.Error(w, `{"error":"payment not found"}`, http.StatusNotFound)
			return
		}
		h.logger.Error("failed to get payment", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	response := models.PaymentResponse{
		PaymentID:   payment.ID.String(),
		Amount:      payment.Amount,
		Currency:    payment.Currency,
		SenderID:    payment.SenderID,
		ReceiverID:  payment.ReceiverID,
		Description: payment.Description,
		Status:      payment.Status,
		CreatedAt:   payment.CreatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// validatePaymentRequest валидирует запрос
func validatePaymentRequest(req models.CreatePaymentRequest) error {
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}
	validCurrencies := map[string]bool{"USD": true, "EUR": true, "RUB": true, "GBP": true}
	if !validCurrencies[req.Currency] {
		return fmt.Errorf("invalid currency, must be USD, EUR, RUB or GBP")
	}
	if req.SenderID == "" {
		return fmt.Errorf("sender_id is required")
	}
	if req.ReceiverID == "" {
		return fmt.Errorf("receiver_id is required")
	}
	if len(req.Description) > 500 {
		return fmt.Errorf("description too long, max 500 characters")
	}
	return nil
}
