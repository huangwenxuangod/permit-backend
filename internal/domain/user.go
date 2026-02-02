package domain

import "time"

type User struct {
	UserID    string `json:"userId"`
	OpenID    string `json:"openid"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
