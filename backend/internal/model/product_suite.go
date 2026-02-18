package model

import "time"

type ProductSuite struct {
	ProdSuiteID            string    `json:"prod_suite_id"`
	ProdSuiteName          string    `json:"prod_suite_name"`
	ProdSuiteOwnerNtAcct   *string   `json:"prod_suite_owner_nt_acct"`
	ProdSuiteSiteOwnerAcct *string   `json:"prod_suite_site_owner_acct"`
	Division               *string   `json:"division"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type CreateProductSuiteRequest struct {
	ProdSuiteName          string  `json:"prod_suite_name" binding:"required"`
	ProdSuiteOwnerNtAcct   *string `json:"prod_suite_owner_nt_acct"`
	ProdSuiteSiteOwnerAcct *string `json:"prod_suite_site_owner_acct"`
	Division               *string `json:"division"`
}
