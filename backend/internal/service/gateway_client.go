package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"wifi-man/backend/internal/domain"
)

type FallbackGatewayController struct {
	primary  GatewayController
	fallback GatewayController
}

func NewFallbackGatewayController(primary, fallback GatewayController) *FallbackGatewayController {
	return &FallbackGatewayController{primary: primary, fallback: fallback}
}

func (c *FallbackGatewayController) Disconnect(ctx context.Context, session domain.Session, reason string) error {
	if c.primary == nil && c.fallback == nil {
		return nil
	}
	if c.primary != nil {
		if err := c.primary.Disconnect(ctx, session, reason); err == nil {
			return nil
		}
	}
	if c.fallback != nil {
		return c.fallback.Disconnect(ctx, session, reason)
	}
	return fmt.Errorf("primary gateway disconnect failed and no fallback configured")
}

type HTTPGatewayController struct {
	baseURL   string
	authToken string
	client    *http.Client
}

func NewHTTPGatewayController(baseURL, authToken string) *HTTPGatewayController {
	return &HTTPGatewayController{
		baseURL:   baseURL,
		authToken: authToken,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *HTTPGatewayController) Disconnect(ctx context.Context, session domain.Session, reason string) error {
	payload := map[string]any{
		"session_id": session.ID,
		"token_id":   session.TokenID,
		"device_mac": session.DeviceMAC,
		"ip_address": session.IPAddress,
		"gateway_id": session.GatewayID,
		"reason":     reason,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gateway disconnect failed with status %d", resp.StatusCode)
	}
	return nil
}

type CoAController struct {
	addr    string
	secret  string
	timeout time.Duration
}

func NewCoAController(addr, secret string, timeout time.Duration) *CoAController {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &CoAController{
		addr:    addr,
		secret:  secret,
		timeout: timeout,
	}
}

func (c *CoAController) Disconnect(ctx context.Context, session domain.Session, reason string) error {
	if c.addr == "" || c.secret == "" {
		return fmt.Errorf("coa not configured")
	}
	conn, err := net.DialTimeout("udp", c.addr, c.timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.timeout))

	attrs := make([]radiusAttr, 0, 6)
	if session.RadiusSessionID != "" {
		attrs = append(attrs, radiusStringAttr(radiusAttrAcctSessionID, session.RadiusSessionID))
	}
	if session.DeviceMAC != "" {
		attrs = append(attrs, radiusStringAttr(radiusAttrCallingStationID, session.DeviceMAC))
	}
	if session.IPAddress != "" {
		if ip := net.ParseIP(session.IPAddress).To4(); ip != nil {
			attrs = append(attrs, radiusIPAttr(radiusAttrFramedIPAddress, ip))
		}
	}
	if reason != "" {
		attrs = append(attrs, radiusStringAttr(radiusAttrReplyMessage, reason))
	}
	if len(attrs) == 0 {
		return fmt.Errorf("no disconnect attributes")
	}

	packet := radiusBuildDisconnectRequest(attrs, c.secret)
	if _, err := conn.Write(packet); err != nil {
		return err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	resp, err := radiusParsePacket(buf[:n])
	if err != nil {
		return err
	}
	switch resp.Code {
	case radiusCodeDisconnectACK:
		return nil
	case radiusCodeDisconnectNAK:
		return fmt.Errorf("coa disconnect rejected")
	default:
		return fmt.Errorf("unexpected coa response code %d", resp.Code)
	}
}

const (
	radiusCodeAccessRequest      = 1
	radiusCodeAccessAccept       = 2
	radiusCodeAccessReject       = 3
	radiusCodeAccountingRequest  = 4
	radiusCodeAccountingResponse = 5
	radiusCodeDisconnectRequest  = 40
	radiusCodeDisconnectACK      = 41
	radiusCodeDisconnectNAK      = 42

	radiusAttrUserName            = 1
	radiusAttrNASIPAddress        = 4
	radiusAttrFramedIPAddress     = 8
	radiusAttrReplyMessage        = 18
	radiusAttrSessionTimeout      = 27
	radiusAttrCallingStationID    = 31
	radiusAttrNASIdentifier       = 32
	radiusAttrAcctStatusType      = 40
	radiusAttrAcctInputOctets     = 42
	radiusAttrAcctOutputOctets    = 43
	radiusAttrAcctSessionID       = 44
	radiusAttrAcctInputGigawords  = 52
	radiusAttrAcctOutputGigawords = 53
	radiusAttrVendorSpecific      = 26
)

type radiusPacket struct {
	Code          uint8
	Identifier    uint8
	Authenticator [16]byte
	Attributes    []radiusAttr
}

type radiusAttr struct {
	Type  uint8
	Value []byte
}

func radiusParsePacket(b []byte) (radiusPacket, error) {
	if len(b) < 20 {
		return radiusPacket{}, fmt.Errorf("short radius packet")
	}
	length := int(binary.BigEndian.Uint16(b[2:4]))
	if length < 20 || length > len(b) {
		return radiusPacket{}, fmt.Errorf("invalid radius packet length")
	}
	var p radiusPacket
	p.Code = b[0]
	p.Identifier = b[1]
	copy(p.Authenticator[:], b[4:20])
	payload := b[20:length]
	for len(payload) > 0 {
		if len(payload) < 2 {
			return radiusPacket{}, fmt.Errorf("invalid attribute header")
		}
		attrLen := int(payload[1])
		if attrLen < 2 || attrLen > len(payload) {
			return radiusPacket{}, fmt.Errorf("invalid attribute length")
		}
		p.Attributes = append(p.Attributes, radiusAttr{
			Type:  payload[0],
			Value: append([]byte(nil), payload[2:attrLen]...),
		})
		payload = payload[attrLen:]
	}
	return p, nil
}

func radiusBuildDisconnectRequest(attrs []radiusAttr, secret string) []byte {
	identifier := make([]byte, 1)
	_, _ = rand.Read(identifier)

	body := make([]byte, 0, 256)
	for _, attr := range attrs {
		body = append(body, attr.Type, uint8(len(attr.Value)+2))
		body = append(body, attr.Value...)
	}
	packetLen := 20 + len(body)
	packet := make([]byte, 0, packetLen+len(secret))
	packet = append(packet, radiusCodeDisconnectRequest, identifier[0], 0, 0)
	packet = append(packet, make([]byte, 16)...)
	packet = append(packet, body...)
	binary.BigEndian.PutUint16(packet[2:4], uint16(packetLen))

	check := make([]byte, 0, len(packet)+len(secret))
	check = append(check, packet[:4]...)
	check = append(check, make([]byte, 16)...)
	check = append(check, packet[20:]...)
	check = append(check, []byte(secret)...)
	sum := md5.Sum(check)
	copy(packet[4:20], sum[:])
	return packet[:packetLen]
}

func radiusStringAttr(t uint8, value string) radiusAttr {
	return radiusAttr{Type: t, Value: []byte(value)}
}

func radiusIPAttr(t uint8, ip net.IP) radiusAttr {
	return radiusAttr{Type: t, Value: ip.To4()}
}
