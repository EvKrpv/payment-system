package models

import (
	"time"

	"github.com/google/uuid"
)

// Payment представляет платёж в системе
type Payment struct {
	ID          uuid.UUID `json:"id"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	SenderID    string    `json:"sender_id"`
	ReceiverID  string    `json:"receiver_id"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreatePaymentRequest — запрос на создание платежа
type CreatePaymentRequest struct {
	Amount      float64 `json:"amount" validate:"required,gt=0"`
	Currency    string  `json:"currency" validate:"required,oneof=USD EUR RUB GBP"`
	SenderID    string  `json:"sender_id" validate:"required"`
	ReceiverID  string  `json:"receiver_id" validate:"required"`
	Description string  `json:"description" validate:"max=500"`
}

// PaymentResponse — ответ с информацией о платеже
type PaymentResponse struct {
	PaymentID   string  `json:"payment_id"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	SenderID    string  `json:"sender_id"`
	ReceiverID  string  `json:"receiver_id"`
	Description string  `json:"description,omitempty"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
}
