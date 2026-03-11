package handlers

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"wifi-man/backend/internal/domain"
	"wifi-man/backend/internal/service"
	"wifi-man/backend/internal/util"
)

type PortalHandler struct {
	tokenService   *service.TokenService
	planService    *service.PlanService
	paymentService *service.PaymentService
	portalTitle    string
}

type PurchaseInput struct {
	PlanID        string `json:"plan_id"`
	DeviceMAC     string `json:"device_mac"`
	Currency      string `json:"currency"`
	Method        string `json:"method"`
	GatewayID     string `json:"gateway_id"`
	CustomerRef   string `json:"customer_ref"`
	LinkLoginOnly string `json:"link_login_only"`
	DST           string `json:"dst"`
	Popup         string `json:"popup"`
}

type PurchaseResult struct {
	Plan      domain.Plan          `json:"plan"`
	Payment   domain.Payment       `json:"payment"`
	TokenCode string               `json:"token_code"`
	Redeem    service.RedeemResult `json:"redeem"`
}

func NewPortalHandler(tokenService *service.TokenService, planService *service.PlanService, paymentService *service.PaymentService, portalTitle string) *PortalHandler {
	return &PortalHandler{tokenService: tokenService, planService: planService, paymentService: paymentService, portalTitle: portalTitle}
}

func (h *PortalHandler) Redeem(c *gin.Context) {
	var req service.RedeemInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DeviceMAC == "" {
		req.DeviceMAC = resolveDeviceMAC(c)
	}
	if normalized, err := util.NormalizeMAC(req.DeviceMAC); err == nil {
		req.DeviceMAC = normalized
	}
	if req.IP == "" {
		req.IP = c.ClientIP()
	}
	if req.GatewayID == "" {
		req.GatewayID = resolveGatewayID(c)
	}
	if req.DeviceMAC == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device MAC address is required"})
		return
	}
	result, err := h.tokenService.RedeemToken(c.Request.Context(), req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"allowed": true, "result": result})
}

func (h *PortalHandler) Resume(c *gin.Context) {
	var req service.RedeemInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DeviceMAC == "" {
		req.DeviceMAC = resolveDeviceMAC(c)
	}
	if normalized, err := util.NormalizeMAC(req.DeviceMAC); err == nil {
		req.DeviceMAC = normalized
	}
	if req.IP == "" {
		req.IP = c.ClientIP()
	}
	if req.GatewayID == "" {
		req.GatewayID = resolveGatewayID(c)
	}
	if req.DeviceMAC == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device MAC address is required"})
		return
	}
	result, err := h.tokenService.ResumeToken(c.Request.Context(), req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"allowed": true, "result": result})
}

func (h *PortalHandler) ListPlans(c *gin.Context) {
	plans, err := h.planService.ListPlans(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"plans": activePlans(plans)})
}

func (h *PortalHandler) PlanCardsFragment(c *gin.Context) {
	plans, err := h.planService.ListPlans(c.Request.Context())
	if err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<div class="empty">Unable to load packages.</div>`))
		return
	}
	deviceMAC := strings.TrimSpace(c.Query("device_mac"))
	if deviceMAC == "" {
		deviceMAC = resolveDeviceMAC(c)
	}
	if normalized, err := util.NormalizeMAC(deviceMAC); err == nil {
		deviceMAC = normalized
	}
	gatewayID := strings.TrimSpace(c.Query("gateway_id"))
	if gatewayID == "" {
		gatewayID = resolveGatewayID(c)
	}
	c.HTML(http.StatusOK, "plan_cards.tmpl", gin.H{
		"plans":           activePlans(plans),
		"detected_mac":    deviceMAC,
		"gateway_id":      gatewayID,
		"link_login_only": resolveLinkLoginOnly(c),
		"dst":             resolveDST(c),
		"popup":           resolvePopup(c),
	})
}

func (h *PortalHandler) Purchase(c *gin.Context) {
	var req PurchaseInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.purchase(c, req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"allowed": true, "result": result})
}

func (h *PortalHandler) Page(c *gin.Context) {
	plans, err := h.planService.ListPlans(c.Request.Context())
	if err != nil {
		c.HTML(http.StatusInternalServerError, "portal.tmpl", gin.H{"title": h.portalTitle, "error": err.Error()})
		return
	}
	c.HTML(http.StatusOK, "portal.tmpl", gin.H{
		"title":           h.portalTitle,
		"plans":           activePlans(plans),
		"detected_mac":    resolveDeviceMAC(c),
		"gateway_id":      resolveGatewayID(c),
		"link_login_only": resolveLinkLoginOnly(c),
		"dst":             resolveDST(c),
		"popup":           resolvePopup(c),
	})
}

func (h *PortalHandler) RedeemFragment(c *gin.Context) {
	code := c.PostForm("token")
	mac := c.PostForm("device_mac")
	if mac == "" {
		mac = resolveDeviceMAC(c)
	}
	if mac == "" {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<div class="result err">Denied: device MAC is required.</div>`))
		return
	}
	normalized, err := util.NormalizeMAC(mac)
	if err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<div class="result err">Denied: invalid device MAC.</div>`))
		return
	}
	ip := c.ClientIP()
	result, err := h.tokenService.RedeemToken(c.Request.Context(), service.RedeemInput{
		Code:      code,
		DeviceMAC: normalized,
		IP:        ip,
		GatewayID: resolveGatewayID(c),
	})
	if err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fmt.Sprintf(`<div class="result err">Denied: %s</div>`, err.Error())))
		return
	}
	message := fmt.Sprintf("Access granted. Session: %s", result.Session.ID)
	tokenSafe := html.EscapeString(code)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderPortalResultHTML(message, true, tokenSafe, resolveLinkLoginOnly(c), resolveDST(c), resolvePopup(c))))
}

func (h *PortalHandler) PurchaseFragment(c *gin.Context) {
	req := PurchaseInput{
		PlanID:        c.PostForm("plan_id"),
		DeviceMAC:     c.PostForm("device_mac"),
		Currency:      c.PostForm("currency"),
		Method:        c.PostForm("method"),
		GatewayID:     c.PostForm("gateway_id"),
		CustomerRef:   c.PostForm("customer_ref"),
		LinkLoginOnly: c.PostForm("link_login_only"),
		DST:           c.PostForm("dst"),
		Popup:         c.PostForm("popup"),
	}
	result, err := h.purchase(c, req)
	if err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fmt.Sprintf(`<div class="result err">Purchase failed: %s</div>`, err.Error())))
		return
	}
	message := fmt.Sprintf("Payment confirmed. Internet activated for %s. Session: %s", result.Redeem.Session.DeviceMAC, result.Redeem.Session.ID)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderPortalResultHTML(message, true, html.EscapeString(result.TokenCode), req.LinkLoginOnly, req.DST, req.Popup)))
}

func (h *PortalHandler) purchase(c *gin.Context, req PurchaseInput) (PurchaseResult, error) {
	result := PurchaseResult{}

	if req.DeviceMAC == "" {
		req.DeviceMAC = resolveDeviceMAC(c)
	}
	normalized, err := util.NormalizeMAC(req.DeviceMAC)
	if err != nil {
		return result, service.ErrInvalidInput
	}
	req.DeviceMAC = normalized
	if req.GatewayID == "" {
		req.GatewayID = resolveGatewayID(c)
	}
	if req.GatewayID == "" {
		req.GatewayID = "portal"
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}
	if req.Method == "" {
		req.Method = "portal_checkout"
	}

	plan, err := h.planService.GetPlan(c.Request.Context(), req.PlanID)
	if err != nil {
		return result, err
	}
	if !plan.Active {
		return result, service.ErrPlanInactive
	}

	payment, _, err := h.paymentService.RecordManualPayment(c.Request.Context(), service.ManualPaymentInput{
		Amount:         plan.Price,
		Currency:       req.Currency,
		Method:         req.Method,
		TransactionRef: "portal-" + uuid.NewString(),
		Metadata: map[string]any{
			"plan_id":      plan.ID,
			"device_mac":   req.DeviceMAC,
			"customer_ref": req.CustomerRef,
		},
		RecordedBy: "portal",
	})
	if err != nil {
		return result, err
	}

	generated, err := h.tokenService.GenerateTokens(c.Request.Context(), service.GenerateTokensInput{
		PlanID:    plan.ID,
		Count:     1,
		CreatedBy: "portal",
		Notes:     "auto-generated after portal purchase",
	})
	if err != nil {
		return result, err
	}

	redeem, err := h.tokenService.RedeemToken(c.Request.Context(), service.RedeemInput{
		Code:      generated[0].Code,
		DeviceMAC: req.DeviceMAC,
		IP:        c.ClientIP(),
		GatewayID: req.GatewayID,
	})
	if err != nil {
		return result, err
	}

	result = PurchaseResult{
		Plan:      plan,
		Payment:   payment,
		TokenCode: generated[0].Code,
		Redeem:    redeem,
	}
	return result, nil
}

func activePlans(plans []domain.Plan) []domain.Plan {
	filtered := make([]domain.Plan, 0, len(plans))
	for _, plan := range plans {
		if plan.Active {
			filtered = append(filtered, plan)
		}
	}
	return filtered
}

func resolveDeviceMAC(c *gin.Context) string {
	candidates := []string{
		c.Query("calling-station-id"),
		c.Query("mac"),
		c.Query("mac-address"),
		c.Query("client_mac"),
		c.Query("device_mac"),
		c.PostForm("device_mac"),
		c.PostForm("mac"),
		c.PostForm("mac-address"),
		c.PostForm("client_mac"),
		c.PostForm("calling-station-id"),
		c.GetHeader("X-Device-MAC"),
		c.GetHeader("X-Calling-Station-Id"),
		c.GetHeader("Calling-Station-Id"),
		c.GetHeader("X-Mikrotik-Mac"),
		c.GetHeader("X-Mikrotik-Calling-Station-Id"),
	}
	for _, value := range candidates {
		if clean, err := util.NormalizeMAC(value); err == nil {
			return clean
		}
	}
	return ""
}

func resolveGatewayID(c *gin.Context) string {
	candidates := []string{
		c.Query("gateway_id"),
		c.Query("ap"),
		c.PostForm("gateway_id"),
		c.GetHeader("X-Gateway-ID"),
	}
	for _, value := range candidates {
		clean := strings.TrimSpace(value)
		if clean != "" {
			return clean
		}
	}
	return ""
}

func resolveLinkLoginOnly(c *gin.Context) string {
	candidates := []string{
		c.Query("link-login-only"),
		c.Query("link_login_only"),
		c.PostForm("link_login_only"),
		c.PostForm("link-login-only"),
		c.GetHeader("X-Link-Login-Only"),
	}
	for _, value := range candidates {
		clean := strings.TrimSpace(value)
		if clean != "" {
			return clean
		}
	}
	return ""
}

func resolveDST(c *gin.Context) string {
	candidates := []string{
		c.Query("dst"),
		c.PostForm("dst"),
	}
	for _, value := range candidates {
		clean := strings.TrimSpace(value)
		if clean != "" {
			return clean
		}
	}
	return ""
}

func resolvePopup(c *gin.Context) string {
	candidates := []string{
		c.Query("popup"),
		c.PostForm("popup"),
	}
	for _, value := range candidates {
		clean := strings.TrimSpace(value)
		if clean != "" {
			return clean
		}
	}
	return ""
}

func renderPortalResultHTML(message string, ok bool, token, linkLoginOnly, dst, popup string) string {
	className := "err"
	if ok {
		className = "ok"
	}
	out := fmt.Sprintf(`<div class="result %s">%s</div>`, className, html.EscapeString(message))
	if linkLoginOnly == "" || token == "" {
		return out
	}
	form := `<form id="mtk-auto-login" method="post" action="%s" style="display:none">
<input type="hidden" name="username" value="%s">
<input type="hidden" name="password" value="">
<input type="hidden" name="dst" value="%s">
<input type="hidden" name="popup" value="%s">
</form>
<script>setTimeout(function(){var f=document.getElementById('mtk-auto-login');if(f){f.submit();}},200);</script>`
	out += fmt.Sprintf(form, html.EscapeString(linkLoginOnly), html.EscapeString(token), html.EscapeString(dst), html.EscapeString(popup))
	return out
}
