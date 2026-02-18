package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/terence/dbmigration-backend/internal/model"
	"github.com/terence/dbmigration-backend/internal/repository"
)

type ProductSuiteService struct {
	repo *repository.ProductSuiteRepository
}

func NewProductSuiteService(repo *repository.ProductSuiteRepository) *ProductSuiteService {
	return &ProductSuiteService{repo: repo}
}

func (s *ProductSuiteService) Create(ctx context.Context, req model.CreateProductSuiteRequest) (*model.ProductSuite, error) {
	ps := &model.ProductSuite{
		ProdSuiteID:            uuid.New().String(),
		ProdSuiteName:          req.ProdSuiteName,
		ProdSuiteOwnerNtAcct:   req.ProdSuiteOwnerNtAcct,
		ProdSuiteSiteOwnerAcct: req.ProdSuiteSiteOwnerAcct,
		Division:               req.Division,
	}

	if err := s.repo.Create(ctx, ps); err != nil {
		return nil, err
	}
	return ps, nil
}
