package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wifi-man/backend/internal/service"
)

type AdminHandler struct {
	tokenService   *service.TokenService
	sessionService *service.SessionService
	paymentService *service.PaymentService
	planService    *service.PlanService
}

func NewAdminHandler(tokenService *service.TokenService, sessionService *service.SessionService, paymentService *service.PaymentService, planService *service.PlanService) *AdminHandler {
	return &AdminHandler{
		tokenService:   tokenService,
		sessionService: sessionService,
		paymentService: paymentService,
		planService:    planService,
	}
}

func (h *AdminHandler) GenerateTokens(c *gin.Context) {
	var req struct {
		PlanID string `json:"plan_id" binding:"required"`
		Count  int    `json:"count" binding:"required"`
		Notes  string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	generated, err := h.tokenService.GenerateTokens(c.Request.Context(), service.GenerateTokensInput{
		PlanID:    req.PlanID,
		Count:     req.Count,
		CreatedBy: c.GetHeader("X-Admin-ID"),
		Notes:     req.Notes,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"tokens": generated})
}

func (h *AdminHandler) RevokeToken(c *gin.Context) {
	tokenID := c.Param("id")
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.tokenService.RevokeToken(c.Request.Context(), tokenID, c.GetHeader("X-Admin-ID"), req.Reason); err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

func (h *AdminHandler) ActiveSessions(c *gin.Context) {
	sessions, err := h.sessionService.ListActiveSessions(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func (h *AdminHandler) SalesReport(c *gin.Context) {
	report, err := h.paymentService.SalesSummary(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h *AdminHandler) ListPlans(c *gin.Context) {
	plans, err := h.planService.ListPlans(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"plans": plans})
}

func (h *AdminHandler) CreatePlan(c *gin.Context) {
	role := c.GetHeader("X-Role")
	if role == "support" {
		c.JSON(http.StatusForbidden, gin.H{"error": "support role cannot create plans"})
		return
	}
	var req service.CreatePlanInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	plan, err := h.planService.CreatePlan(c.Request.Context(), req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"plan": plan})
}
