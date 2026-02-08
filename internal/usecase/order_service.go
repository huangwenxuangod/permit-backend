package usecase

import (
	"encoding/json"
	"permit-backend/internal/domain"
	"strconv"
	"time"
)

type OrderRepo interface {
	Put(*domain.Order) error
	Get(id string) (*domain.Order, bool)
	List(page, pageSize int) ([]domain.Order, int)
}

type OrderService struct {
	Repo        OrderRepo
	PayMock     bool
	WechatAppID string
}

func (s *OrderService) Create(req *domain.Order) (string, error) {
	id := randomID()
	now := time.Now().UTC()
	req.OrderID = id
	req.Status = domain.OrderCreated
	req.PayIdempotencyKey = ""
	req.PayParams = ""
	req.CreatedAt = now
	req.UpdatedAt = now
	_ = s.Repo.Put(req)
	return id, nil
}

func (s *OrderService) Pay(orderID, channel, idempotencyKey string) (map[string]any, error) {
	o, ok := s.Repo.Get(orderID)
	if !ok {
		return nil, ErrNotFound("order")
	}
	if o.Status == domain.OrderPaid {
		return nil, ErrConflict("order already paid")
	}
	if o.PayIdempotencyKey != "" && o.PayIdempotencyKey != idempotencyKey {
		return nil, ErrConflict("idempotency key mismatch")
	}
	if o.PayIdempotencyKey == idempotencyKey && o.PayParams != "" {
		var cached map[string]any
		if err := json.Unmarshal([]byte(o.PayParams), &cached); err == nil {
			return cached, nil
		}
	}
	o.Channel = channel
	o.Status = domain.OrderPending
	o.PayIdempotencyKey = idempotencyKey
	o.UpdatedAt = time.Now().UTC()
	prepayID := "mock-" + randomID()
	p := map[string]any{
		"appId":     s.WechatAppID,
		"timeStamp": strconv.FormatInt(time.Now().Unix(), 10),
		"nonceStr":  randomID(),
		"package":   "prepay_id=" + prepayID,
		"signType":  "RSA",
		"paySign":   "MOCK_SIGN",
	}
	raw, _ := json.Marshal(p)
	o.PayParams = string(raw)
	_ = s.Repo.Put(o)
	return p, nil
}

func (s *OrderService) Callback(orderID, status string) error {
	o, ok := s.Repo.Get(orderID)
	if !ok {
		return ErrNotFound("order")
	}
	switch status {
	case "paid":
		o.Status = domain.OrderPaid
	case "pending":
		o.Status = domain.OrderPending
	case "canceled":
		o.Status = domain.OrderCanceled
	case "refunded":
		o.Status = domain.OrderRefunded
	default:
		return ErrBadRequest("invalid status")
	}
	o.UpdatedAt = time.Now().UTC()
	_ = s.Repo.Put(o)
	return nil
}

type ErrNotFound string

func (e ErrNotFound) Error() string { return string(e) + " not found" }

type ErrConflict string

func (e ErrConflict) Error() string { return string(e) }

type ErrBadRequest string

func (e ErrBadRequest) Error() string { return string(e) }
