package products

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidInput = errors.New("invalid product data")
	ErrSKUExists    = errors.New("SKU already exists")
	ErrNotFound     = errors.New("product not found")
)

type Product struct {
	ID                int64     `json:"id"`
	SKU               string    `json:"sku"`
	Name              string    `json:"name"`
	Description       *string   `json:"description"`
	IsActive          bool      `json:"is_active"`
	LowStockThreshold int64     `json:"low_stock_threshold"`
	OnHand            int64     `json:"on_hand"`
	Reserved          int64     `json:"reserved"`
	Available         int64     `json:"available"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CreateInput struct {
	SKU               string  `json:"sku"`
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	LowStockThreshold int64   `json:"low_stock_threshold"`
}

type UpdateInput struct {
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	LowStockThreshold int64   `json:"low_stock_threshold"`
}

type Page struct {
	Items   []Product `json:"items"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
	HasMore bool      `json:"has_more"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func ValidateCreate(in CreateInput) error {
	if !validSKU(in.SKU) {
		return fmt.Errorf("%w: invalid SKU", ErrInvalidInput)
	}

	return validateDetails(
		in.Name,
		in.Description,
		in.LowStockThreshold,
	)
}

func ValidateUpdate(in UpdateInput) error {
	return validateDetails(
		in.Name,
		in.Description,
		in.LowStockThreshold,
	)
}

func validSKU(value string) bool {
	n := utf8.RuneCountInString(value)

	if !utf8.ValidString(value) ||
		n < 3 || n > 64 ||
		strings.TrimSpace(value) != value {
		return false
	}

	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}

	return true
}

func validateDetails(
	name string,
	description *string,
	threshold int64,
) error {
	n := utf8.RuneCountInString(name)

	if !utf8.ValidString(name) ||
		n < 1 || n > 200 ||
		strings.TrimSpace(name) != name ||
		threshold < 0 {
		return ErrInvalidInput
	}

	for _, r := range name {
		if unicode.IsControl(r) {
			return ErrInvalidInput
		}
	}

	if description != nil {
		if !utf8.ValidString(*description) ||
			utf8.RuneCountInString(*description) > 2000 ||
			strings.ContainsRune(*description, '\x00') {
			return ErrInvalidInput
		}
	}

	return nil
}

type queryRower interface {
	QueryRowContext(
		context.Context,
		string,
		...any,
	) *sql.Row
}

type scanner interface {
	Scan(...any) error
}

const productColumns = `
	p.id,
	p.sku,
	p.name,
	p.description,
	p.is_active,
	p.low_stock_threshold,
	b.on_hand,
	b.reserved,
	p.created_at,
	p.updated_at
`

func scanProduct(row scanner) (Product, error) {
	var p Product
	var description sql.NullString

	err := row.Scan(
		&p.ID,
		&p.SKU,
		&p.Name,
		&description,
		&p.IsActive,
		&p.LowStockThreshold,
		&p.OnHand,
		&p.Reserved,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		return Product{}, err
	}

	if description.Valid {
		value := description.String
		p.Description = &value
	}

	p.Available = p.OnHand - p.Reserved

	return p, nil
}

func getProduct(
	ctx context.Context,
	db queryRower,
	id int64,
) (Product, error) {
	if id <= 0 {
		return Product{}, ErrNotFound
	}

	query := `
		SELECT ` + productColumns + `
		FROM products AS p
		JOIN inventory_balances AS b
		  ON b.product_id = p.id
		WHERE p.id = $1
	`

	p, err := scanProduct(
		db.QueryRowContext(ctx, query, id),
	)

	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, ErrNotFound
	}

	if err != nil {
		return Product{}, fmt.Errorf(
			"get product: %w", err,
		)
	}

	return p, nil
}

func (s *Store) Get(
	ctx context.Context,
	id int64,
) (Product, error) {
	return getProduct(ctx, s.db, id)
}

func (s *Store) List(
	ctx context.Context,
	limit int,
	offset int,
	includeInactive bool,
) (Page, error) {
	if limit < 1 || limit > 100 ||
		offset < 0 || offset > 1000000 {
		return Page{}, ErrInvalidInput
	}

	query := `
		SELECT ` + productColumns + `
		FROM products AS p
		JOIN inventory_balances AS b
		  ON b.product_id = p.id
		WHERE ($1 OR p.is_active = TRUE)
		ORDER BY p.id ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(
		ctx,
		query,
		includeInactive,
		limit+1,
		offset,
	)

	if err != nil {
		return Page{}, fmt.Errorf(
			"list products: %w", err,
		)
	}
	defer rows.Close()

	page := Page{
		Items:  make([]Product, 0),
		Limit:  limit,
		Offset: offset,
	}

	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return Page{}, fmt.Errorf(
				"scan product: %w", err,
			)
		}

		if len(page.Items) == limit {
			page.HasMore = true
			break
		}

		page.Items = append(page.Items, p)
	}

	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf(
			"iterate products: %w", err,
		)
	}

	return page, nil
}

// Create inserts the product and zero inventory balance
// atomically. No inventory movement is necessary because
// the initial balance is zero.
func (s *Store) Create(
	ctx context.Context,
	input CreateInput,
) (Product, error) {
	if err := ValidateCreate(input); err != nil {
		return Product{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Product{}, fmt.Errorf(
			"begin product creation: %w", err,
		)
	}
	defer tx.Rollback()

	var id int64

	err = tx.QueryRowContext(ctx, `
		INSERT INTO products (
			sku,
			name,
			description,
			low_stock_threshold
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
		RETURNING id
	`,
		input.SKU,
		input.Name,
		input.Description,
		input.LowStockThreshold,
	).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, ErrSKUExists
	}

	if err != nil {
		return Product{}, fmt.Errorf(
			"insert product: %w", err,
		)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_balances (
			product_id,
			on_hand,
			reserved
		)
		VALUES ($1, 0, 0)
	`, id)

	if err != nil {
		return Product{}, fmt.Errorf(
			"initialize inventory balance: %w",
			err,
		)
	}

	p, err := getProduct(ctx, tx, id)
	if err != nil {
		return Product{}, err
	}

	if err := tx.Commit(); err != nil {
		return Product{}, fmt.Errorf(
			"commit product creation: %w", err,
		)
	}

	return p, nil
}

// Update changes product metadata only.
// SKU and inventory quantities remain unchanged.
func (s *Store) Update(
	ctx context.Context,
	id int64,
	input UpdateInput,
) (Product, error) {
	if id <= 0 {
		return Product{}, ErrNotFound
	}

	if err := ValidateUpdate(input); err != nil {
		return Product{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Product{}, fmt.Errorf(
			"begin product update: %w", err,
		)
	}
	defer tx.Rollback()

	var updatedID int64

	err = tx.QueryRowContext(ctx, `
		UPDATE products
		SET
			name = $2,
			description = $3,
			low_stock_threshold = $4,
			updated_at = statement_timestamp()
		WHERE id = $1
		  AND is_active = TRUE
		RETURNING id
	`,
		id,
		input.Name,
		input.Description,
		input.LowStockThreshold,
	).Scan(&updatedID)

	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, ErrNotFound
	}

	if err != nil {
		return Product{}, fmt.Errorf(
			"update product: %w", err,
		)
	}

	p, err := getProduct(ctx, tx, updatedID)
	if err != nil {
		return Product{}, err
	}

	if err := tx.Commit(); err != nil {
		return Product{}, fmt.Errorf(
			"commit product update: %w", err,
		)
	}

	return p, nil
}

// Deactivation is idempotent and keeps historical data.
func (s *Store) Deactivate(
	ctx context.Context,
	id int64,
) error {
	if id <= 0 {
		return ErrNotFound
	}

	var updatedID int64

	err := s.db.QueryRowContext(ctx, `
		UPDATE products
		SET
			is_active = FALSE,
			updated_at = CASE
				WHEN is_active THEN statement_timestamp()
				ELSE updated_at
			END
		WHERE id = $1
		RETURNING id
	`, id).Scan(&updatedID)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}

	if err != nil {
		return fmt.Errorf(
			"deactivate product: %w", err,
		)
	}

	return nil
}
