package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wifi-man/backend/internal/service"
)

type GatewayHandler struct {
	gatewayService *service.GatewayIntegrationService
}

func NewGatewayHandler(gatewayService *service.GatewayIntegrationService) *GatewayHandler {
	return &GatewayHandler{gatewayService: gatewayService}
}

func (h *GatewayHandler) Disconnect(c *gin.Context) {
	var req service.DisconnectInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	session, err := h.gatewayService.DisconnectSession(c.Request.Context(), req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": session})
}
