package domain

import "time"

type OrderStatus string

const (
	OrderCreated  OrderStatus = "created"
	OrderPending  OrderStatus = "pending"
	OrderPaid     OrderStatus = "paid"
	OrderCanceled OrderStatus = "canceled"
	OrderRefunded OrderStatus = "refunded"
)

type OrderItem struct {
	Type string `json:"type"`
	Qty  int    `json:"qty"`
}

type Order struct {
	OrderID           string      `json:"orderId"`
	TaskID            string      `json:"taskId"`
	Items             []OrderItem `json:"items"`
	City              string      `json:"city"`
	Remark            string      `json:"remark"`
	AmountCents       int         `json:"amountCents"`
	Channel           string      `json:"channel"`
	Status            OrderStatus `json:"status"`
	PayIdempotencyKey string      `json:"-"`
	PayParams         string      `json:"-"`
	CreatedAt         time.Time   `json:"createdAt"`
	UpdatedAt         time.Time   `json:"updatedAt"`
}
