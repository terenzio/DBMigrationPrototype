package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/terence/dbmigration-backend/internal/model"
	"github.com/terence/dbmigration-backend/internal/service"
)

type ProductSuiteHandler struct {
	svc *service.ProductSuiteService
}

func NewProductSuiteHandler(svc *service.ProductSuiteService) *ProductSuiteHandler {
	return &ProductSuiteHandler{svc: svc}
}

func (h *ProductSuiteHandler) Create(c *gin.Context) {
	var req model.CreateProductSuiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create product suite"})
		return
	}

	c.JSON(http.StatusCreated, result)
}
