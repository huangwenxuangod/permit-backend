package repo

import (
	"database/sql"
	"encoding/json"
	_ "github.com/lib/pq"
	"permit-backend/internal/domain"
)

type PostgresRepo struct {
	db *sql.DB
}

func NewPostgresRepo(dsn string) (*PostgresRepo, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	r := &PostgresRepo{db: db}
	if err := r.init(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *PostgresRepo) init() error {
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		spec_code TEXT,
		source_object_key TEXT,
		status TEXT,
		error_msg TEXT,
		processed_urls TEXT,
		created_at TIMESTAMPTZ,
		updated_at TIMESTAMPTZ
	);`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`CREATE TABLE IF NOT EXISTS users (
		user_id TEXT PRIMARY KEY,
		openid TEXT UNIQUE,
		nickname TEXT,
		avatar TEXT,
		created_at TIMESTAMPTZ,
		updated_at TIMESTAMPTZ
	);`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`CREATE TABLE IF NOT EXISTS orders (
		order_id TEXT PRIMARY KEY,
		task_id TEXT,
		items TEXT,
		city TEXT,
		remark TEXT,
		amount_cents INT,
		channel TEXT,
		status TEXT,
		pay_idempotency_key TEXT,
		pay_params TEXT,
		created_at TIMESTAMPTZ,
		updated_at TIMESTAMPTZ
	);`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS pay_idempotency_key TEXT;`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`ALTER TABLE orders ADD COLUMN IF NOT EXISTS pay_params TEXT;`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`CREATE TABLE IF NOT EXISTS specs (
		code TEXT PRIMARY KEY,
		name TEXT,
		width_px INT,
		height_px INT,
		dpi INT,
		bg_colors TEXT
	);`)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`CREATE TABLE IF NOT EXISTS download_tokens (
		token TEXT PRIMARY KEY,
		task_id TEXT,
		user_id TEXT,
		status TEXT,
		expires_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ,
		used_at TIMESTAMPTZ
	);`)
	return err
}

func (r *PostgresRepo) PutUser(u *domain.User) error {
	_, err := r.db.Exec(`INSERT INTO users (user_id,openid,nickname,avatar,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (openid) DO UPDATE SET nickname=EXCLUDED.nickname, avatar=EXCLUDED.avatar, updated_at=EXCLUDED.updated_at`, u.UserID, u.OpenID, u.Nickname, u.Avatar, u.CreatedAt, u.UpdatedAt)
	return err
}

func (r *PostgresRepo) GetUserByOpenID(openid string) (*domain.User, bool) {
	var u domain.User
	err := r.db.QueryRow(`SELECT user_id,openid,nickname,avatar,created_at,updated_at FROM users WHERE openid=$1`, openid).
		Scan(&u.UserID, &u.OpenID, &u.Nickname, &u.Avatar, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, false
	}
	return &u, true
}

func (r *PostgresRepo) Put(t *domain.Task) error {
	pUrls, _ := json.Marshal(t.ProcessedUrls)
	_, err := r.db.Exec(`INSERT INTO tasks (id,user_id,spec_code,source_object_key,status,error_msg,processed_urls,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET user_id=$2,spec_code=$3,source_object_key=$4,status=$5,error_msg=$6,processed_urls=$7,updated_at=$9`,
		t.ID, t.UserID, t.SpecCode, t.SourceObjectKey, string(t.Status), t.ErrorMsg, string(pUrls), t.CreatedAt, t.UpdatedAt)
	return err
}

func (r *PostgresRepo) Get(id string) (*domain.Task, bool) {
	var t domain.Task
	var pUrls string
	err := r.db.QueryRow(`SELECT id,user_id,spec_code,source_object_key,status,error_msg,processed_urls,created_at,updated_at FROM tasks WHERE id=$1`, id).
		Scan(&t.ID, &t.UserID, &t.SpecCode, &t.SourceObjectKey, (*string)(&t.Status), &t.ErrorMsg, &pUrls, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, false
	}
	_ = json.Unmarshal([]byte(pUrls), &t.ProcessedUrls)
	if t.ProcessedUrls == nil {
		t.ProcessedUrls = map[string]string{}
	}
	return &t, true
}

func (r *PostgresRepo) PutOrder(o *domain.Order) error {
	items, _ := json.Marshal(o.Items)
	_, err := r.db.Exec(`INSERT INTO orders (order_id,task_id,items,city,remark,amount_cents,channel,status,pay_idempotency_key,pay_params,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (order_id) DO UPDATE SET task_id=$2,items=$3,city=$4,remark=$5,amount_cents=$6,channel=$7,status=$8,pay_idempotency_key=$9,pay_params=$10,updated_at=$12`,
		o.OrderID, o.TaskID, string(items), o.City, o.Remark, o.AmountCents, o.Channel, string(o.Status), o.PayIdempotencyKey, o.PayParams, o.CreatedAt, o.UpdatedAt)
	return err
}

func (r *PostgresRepo) GetOrder(id string) (*domain.Order, bool) {
	var o domain.Order
	var items string
	err := r.db.QueryRow(`SELECT order_id,task_id,items,city,remark,amount_cents,channel,status,pay_idempotency_key,pay_params,created_at,updated_at FROM orders WHERE order_id=$1`, id).
		Scan(&o.OrderID, &o.TaskID, &items, &o.City, &o.Remark, &o.AmountCents, &o.Channel, (*string)(&o.Status), &o.PayIdempotencyKey, &o.PayParams, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, false
	}
	_ = json.Unmarshal([]byte(items), &o.Items)
	return &o, true
}

func (r *PostgresRepo) ListOrders(page, pageSize int) ([]domain.Order, int) {
	rows, err := r.db.Query(`SELECT order_id,task_id,items,city,remark,amount_cents,channel,status,pay_idempotency_key,pay_params,created_at,updated_at FROM orders ORDER BY created_at DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()
	out := make([]domain.Order, 0, pageSize)
	for rows.Next() {
		var o domain.Order
		var items string
		_ = rows.Scan(&o.OrderID, &o.TaskID, &items, &o.City, &o.Remark, &o.AmountCents, &o.Channel, (*string)(&o.Status), &o.PayIdempotencyKey, &o.PayParams, &o.CreatedAt, &o.UpdatedAt)
		_ = json.Unmarshal([]byte(items), &o.Items)
		out = append(out, o)
	}
	var total int
	_ = r.db.QueryRow(`SELECT COUNT(1) FROM orders`).Scan(&total)
	return out, total
}

func (r *PostgresRepo) UpsertSpecs(specs []domain.SpecDef) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO specs (code,name,width_px,height_px,dpi,bg_colors)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (code) DO UPDATE SET name=$2,width_px=$3,height_px=$4,dpi=$5,bg_colors=$6`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range specs {
		bg, _ := json.Marshal(s.BgColors)
		if _, err := stmt.Exec(s.Code, s.Name, s.WidthPx, s.HeightPx, s.DPI, string(bg)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *PostgresRepo) ListSpecs() ([]domain.SpecDef, error) {
	rows, err := r.db.Query(`SELECT code,name,width_px,height_px,dpi,bg_colors FROM specs ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SpecDef
	for rows.Next() {
		var s domain.SpecDef
		var bg string
		if err := rows.Scan(&s.Code, &s.Name, &s.WidthPx, &s.HeightPx, &s.DPI, &bg); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(bg), &s.BgColors)
		out = append(out, s)
	}
	return out, nil
}

func (r *PostgresRepo) PutToken(tk *domain.DownloadToken) error {
	var usedAt sql.NullTime
	if !tk.UsedAt.IsZero() {
		usedAt = sql.NullTime{Time: tk.UsedAt, Valid: true}
	}
	_, err := r.db.Exec(`INSERT INTO download_tokens (token,task_id,user_id,status,expires_at,created_at,used_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (token) DO UPDATE SET task_id=$2,user_id=$3,status=$4,expires_at=$5,created_at=$6,used_at=$7`,
		tk.Token, tk.TaskID, tk.UserID, string(tk.Status), tk.ExpiresAt, tk.CreatedAt, usedAt)
	return err
}

func (r *PostgresRepo) GetToken(token string) (*domain.DownloadToken, bool) {
	var tk domain.DownloadToken
	var status string
	var usedAt sql.NullTime
	err := r.db.QueryRow(`SELECT token,task_id,user_id,status,expires_at,created_at,used_at FROM download_tokens WHERE token=$1`, token).
		Scan(&tk.Token, &tk.TaskID, &tk.UserID, &status, &tk.ExpiresAt, &tk.CreatedAt, &usedAt)
	if err != nil {
		return nil, false
	}
	tk.Status = domain.DownloadTokenStatus(status)
	if usedAt.Valid {
		tk.UsedAt = usedAt.Time
	}
	return &tk, true
}

func (r *PostgresRepo) UpdateToken(tk *domain.DownloadToken) error {
	var usedAt sql.NullTime
	if !tk.UsedAt.IsZero() {
		usedAt = sql.NullTime{Time: tk.UsedAt, Valid: true}
	}
	_, err := r.db.Exec(`UPDATE download_tokens SET status=$2,expires_at=$3,created_at=$4,used_at=$5 WHERE token=$1`,
		tk.Token, string(tk.Status), tk.ExpiresAt, tk.CreatedAt, usedAt)
	return err
}
