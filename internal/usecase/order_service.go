package usecase

import (
	"time"
	"permit-backend/internal/domain"
	"strconv"
)

type OrderRepo interface {
	Put(*domain.Order) error
	Get(id string) (*domain.Order, bool)
	List(page, pageSize int) ([]domain.Order, int)
}

type OrderService struct {
	Repo OrderRepo
	PayMock bool
	WechatAppID string
}

func (s *OrderService) Create(req *domain.Order) (string, error) {
	id := randomID()
	now := time.Now().UTC()
	req.OrderID = id
	req.Status = domain.OrderCreated
	req.CreatedAt = now
	req.UpdatedAt = now
	_ = s.Repo.Put(req)
	return id, nil
}

func (s *OrderService) Pay(orderID, channel string) (map[string]any, error) {
	o, ok := s.Repo.Get(orderID)
	if !ok {
		return nil, ErrNotFound("order")
	}
	o.Channel = channel
	o.Status = domain.OrderPending
	o.UpdatedAt = time.Now().UTC()
	_ = s.Repo.Put(o)
	prepayID := "mock-" + randomID()
	p := map[string]any{
		"appId":     s.WechatAppID,
		"timeStamp": strconv.FormatInt(time.Now().Unix(), 10),
		"nonceStr":  randomID(),
		"package":   "prepay_id=" + prepayID,
		"signType":  "RSA",
		"paySign":   "MOCK_SIGN",
	}
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
	}
	o.UpdatedAt = time.Now().UTC()
	_ = s.Repo.Put(o)
	return nil
}

type ErrNotFound string
func (e ErrNotFound) Error() string { return string(e) + " not found" }
