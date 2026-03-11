package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"wifi-man/backend/internal/service"
)

type PaymentHandler struct {
	paymentService *service.PaymentService
}

func NewPaymentHandler(paymentService *service.PaymentService) *PaymentHandler {
	return &PaymentHandler{paymentService: paymentService}
}

func (h *PaymentHandler) ManualRecord(c *gin.Context) {
	var req service.ManualPaymentInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	payment, existed, err := h.paymentService.RecordManualPayment(c.Request.Context(), req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	status := http.StatusCreated
	if existed {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"payment": payment, "duplicate": existed})
}

func (h *PaymentHandler) MobileMoneyWebhook(c *gin.Context) {
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req service.MobileMoneyWebhookInput
	if err := json.Unmarshal(rawBody, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.RawBody = rawBody
	req.Signature = c.GetHeader("X-Signature")
	payment, existed, err := h.paymentService.ProcessMobileMoneyWebhook(c.Request.Context(), req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"payment": payment, "duplicate": existed})
}
