package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wifi-man/backend/internal/service"
)

type AccountingHandler struct {
	sessionService *service.SessionService
	gatewayService *service.GatewayIntegrationService
}

func NewAccountingHandler(sessionService *service.SessionService, gatewayService *service.GatewayIntegrationService) *AccountingHandler {
	return &AccountingHandler{sessionService: sessionService, gatewayService: gatewayService}
}

func (h *AccountingHandler) Start(c *gin.Context) {
	var req service.AccountingStartInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	session, err := h.sessionService.StartAccounting(c.Request.Context(), req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": session})
}

func (h *AccountingHandler) Interim(c *gin.Context) {
	var req service.AccountingInterimInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := h.sessionService.InterimAccounting(c.Request.Context(), req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if res.Disconnect && h.gatewayService != nil {
		_, err = h.gatewayService.DisconnectSession(c.Request.Context(), service.DisconnectInput{
			SessionID: req.SessionID,
			Reason:    res.DisconnectReason,
			Force:     true,
		})
		if err != nil {
			writeServiceError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, res)
}

func (h *AccountingHandler) Stop(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
		Reason    string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	session, err := h.sessionService.StopAccounting(c.Request.Context(), req.SessionID, req.Reason)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": session})
}
