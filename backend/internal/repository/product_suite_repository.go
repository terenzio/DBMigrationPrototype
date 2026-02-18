package repository

import (
	"context"
	"database/sql"

	"github.com/terence/dbmigration-backend/internal/model"
)

type ProductSuiteRepository struct {
	db *sql.DB
}

func NewProductSuiteRepository(db *sql.DB) *ProductSuiteRepository {
	return &ProductSuiteRepository{db: db}
}

func (r *ProductSuiteRepository) Create(ctx context.Context, ps *model.ProductSuite) error {
	query := `INSERT INTO product_suites_info
		(prod_suite_id, prod_suite_name, prod_suite_owner_nt_acct,
		 prod_suite_site_owner_acct, division)
		VALUES (:1, :2, :3, :4, :5)`

	_, err := r.db.ExecContext(ctx, query,
		ps.ProdSuiteID,
		ps.ProdSuiteName,
		ps.ProdSuiteOwnerNtAcct,
		ps.ProdSuiteSiteOwnerAcct,
		ps.Division,
	)
	return err
}
