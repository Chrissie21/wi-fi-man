package service

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

type RadiusServer struct {
	authConn *net.UDPConn
	acctConn *net.UDPConn
	secret   string
	service  *RadiusService
	cancel   context.CancelFunc
}

func NewRadiusServer(authAddr, acctAddr, secret string, svc *RadiusService) (*RadiusServer, error) {
	authUDP, err := net.ResolveUDPAddr("udp", authAddr)
	if err != nil {
		return nil, err
	}
	acctUDP, err := net.ResolveUDPAddr("udp", acctAddr)
	if err != nil {
		return nil, err
	}
	authConn, err := net.ListenUDP("udp", authUDP)
	if err != nil {
		return nil, err
	}
	acctConn, err := net.ListenUDP("udp", acctUDP)
	if err != nil {
		_ = authConn.Close()
		return nil, err
	}
	return &RadiusServer{
		authConn: authConn,
		acctConn: acctConn,
		secret:   secret,
		service:  svc,
	}, nil
}

func (s *RadiusServer) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	go s.serveAuth(ctx)
	go s.serveAccounting(ctx)
}

func (s *RadiusServer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.authConn != nil {
		_ = s.authConn.Close()
	}
	if s.acctConn != nil {
		_ = s.acctConn.Close()
	}
}

func (s *RadiusServer) serveAuth(ctx context.Context) {
	buf := make([]byte, 4096)
	for {
		_ = s.authConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := s.authConn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("radius auth read error: %v", err)
			continue
		}
		raw := append([]byte(nil), buf[:n]...)
		go func(pkt []byte, remote *net.UDPAddr) {
			resp, err := s.handleAuthPacket(ctx, pkt, remote)
			if err != nil {
				log.Printf("radius auth packet error: %v", err)
				return
			}
			if len(resp) == 0 {
				return
			}
			if _, err := s.authConn.WriteToUDP(resp, remote); err != nil {
				log.Printf("radius auth write error: %v", err)
			}
		}(raw, addr)
	}
}

func (s *RadiusServer) serveAccounting(ctx context.Context) {
	buf := make([]byte, 4096)
	for {
		_ = s.acctConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := s.acctConn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("radius accounting read error: %v", err)
			continue
		}
		raw := append([]byte(nil), buf[:n]...)
		go func(pkt []byte, remote *net.UDPAddr) {
			resp, err := s.handleAccountingPacket(ctx, pkt, remote)
			if err != nil {
				log.Printf("radius accounting packet error: %v", err)
			}
			if len(resp) == 0 {
				return
			}
			if _, err := s.acctConn.WriteToUDP(resp, remote); err != nil {
				log.Printf("radius accounting write error: %v", err)
			}
		}(raw, addr)
	}
}

func (s *RadiusServer) handleAuthPacket(ctx context.Context, raw []byte, remote *net.UDPAddr) ([]byte, error) {
	packet, err := radiusParsePacket(raw)
	if err != nil {
		return nil, err
	}
	if packet.Code != radiusCodeAccessRequest {
		return nil, nil
	}
	in := RadiusAuthInput{
		UserName:         packet.firstString(radiusAttrUserName),
		CallingStationID: packet.firstString(radiusAttrCallingStationID),
		FramedIP:         firstNonEmpty(packet.firstIP(radiusAttrFramedIPAddress), remote.IP.String()),
		NASIP:            packet.firstIP(radiusAttrNASIPAddress),
		NASIdentifier:    packet.firstString(radiusAttrNASIdentifier),
		AcctSessionID:    packet.firstString(radiusAttrAcctSessionID),
	}

	decision, err := s.service.Authenticate(ctx, in)
	if err != nil {
		return nil, err
	}
	if decision.Allow {
		rate := decision.Policy.MikrotikRateLimitVSA
		if rate == "" {
			rate = fmt.Sprintf("%dk/%dk", decision.Policy.SpeedDownKbps, decision.Policy.SpeedUpKbps)
		}
		attrs := []radiusAttr{
			radiusUint32Attr(radiusAttrSessionTimeout, uint32(max(decision.Policy.SessionTimeoutSec, 60))),
			radiusMikrotikRateLimitAttr(rate),
			radiusStringAttr(radiusAttrReplyMessage, "Access granted"),
		}
		return radiusBuildResponse(packet, radiusCodeAccessAccept, attrs, s.secret), nil
	}
	reason := decision.Reason
	if reason == "" {
		reason = "access denied"
	}
	attrs := []radiusAttr{
		radiusStringAttr(radiusAttrReplyMessage, reason),
	}
	return radiusBuildResponse(packet, radiusCodeAccessReject, attrs, s.secret), nil
}

func (s *RadiusServer) handleAccountingPacket(ctx context.Context, raw []byte, remote *net.UDPAddr) ([]byte, error) {
	packet, err := radiusParsePacket(raw)
	if err != nil {
		return nil, err
	}
	if packet.Code != radiusCodeAccountingRequest {
		return nil, nil
	}
	if !radiusValidateRequest(raw, s.secret) {
		return nil, fmt.Errorf("invalid accounting authenticator")
	}

	status := packet.firstUint32(radiusAttrAcctStatusType)
	statusType := ""
	switch status {
	case 1:
		statusType = "start"
	case 2:
		statusType = "stop"
	case 3:
		statusType = "interim"
	default:
		statusType = "interim"
	}
	nasIP := packet.firstIP(radiusAttrNASIPAddress)
	if nasIP == "" {
		nasIP = remote.IP.String()
	}

	in := RadiusAccountingEvent{
		StatusType:       statusType,
		UserName:         packet.firstString(radiusAttrUserName),
		CallingStationID: packet.firstString(radiusAttrCallingStationID),
		FramedIP:         packet.firstIP(radiusAttrFramedIPAddress),
		NASIP:            nasIP,
		NASIdentifier:    packet.firstString(radiusAttrNASIdentifier),
		AcctSessionID:    packet.firstString(radiusAttrAcctSessionID),
		BytesInTotal:     packet.octets(radiusAttrAcctInputOctets, radiusAttrAcctInputGigawords),
		BytesOutTotal:    packet.octets(radiusAttrAcctOutputOctets, radiusAttrAcctOutputGigawords),
		StopReason:       packet.firstString(radiusAttrReplyMessage),
	}
	if err := s.service.HandleAccounting(ctx, in); err != nil {
		return nil, err
	}
	return radiusBuildResponse(packet, radiusCodeAccountingResponse, nil, s.secret), nil
}

func radiusBuildResponse(req radiusPacket, code uint8, attrs []radiusAttr, secret string) []byte {
	body := make([]byte, 0, 256)
	for _, attr := range attrs {
		if len(attr.Value) > 253 {
			continue
		}
		body = append(body, attr.Type, uint8(len(attr.Value)+2))
		body = append(body, attr.Value...)
	}
	packetLen := 20 + len(body)
	packet := make([]byte, 0, packetLen+len(secret))
	packet = append(packet, code, req.Identifier, 0, 0)
	packet = append(packet, req.Authenticator[:]...)
	packet = append(packet, body...)
	binary.BigEndian.PutUint16(packet[2:4], uint16(packetLen))

	check := make([]byte, 0, len(packet)+len(secret))
	check = append(check, packet[:4]...)
	check = append(check, req.Authenticator[:]...)
	check = append(check, packet[20:]...)
	check = append(check, []byte(secret)...)
	sum := md5.Sum(check)
	copy(packet[4:20], sum[:])
	return packet[:packetLen]
}

func radiusValidateRequest(raw []byte, secret string) bool {
	if len(raw) < 20 {
		return false
	}
	length := int(binary.BigEndian.Uint16(raw[2:4]))
	if length < 20 || length > len(raw) {
		return false
	}
	packet := raw[:length]
	check := make([]byte, 0, len(packet)+len(secret))
	check = append(check, packet[:4]...)
	check = append(check, make([]byte, 16)...)
	check = append(check, packet[20:]...)
	check = append(check, []byte(secret)...)
	sum := md5.Sum(check)
	return strings.EqualFold(fmt.Sprintf("%x", sum[:]), fmt.Sprintf("%x", packet[4:20]))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (p radiusPacket) firstString(attrType uint8) string {
	for _, attr := range p.Attributes {
		if attr.Type == attrType {
			return strings.TrimSpace(string(attr.Value))
		}
	}
	return ""
}

func (p radiusPacket) firstIP(attrType uint8) string {
	for _, attr := range p.Attributes {
		if attr.Type != attrType {
			continue
		}
		if len(attr.Value) != 4 {
			continue
		}
		return net.IP(attr.Value).String()
	}
	return ""
}

func (p radiusPacket) firstUint32(attrType uint8) uint32 {
	for _, attr := range p.Attributes {
		if attr.Type != attrType {
			continue
		}
		if len(attr.Value) != 4 {
			continue
		}
		return binary.BigEndian.Uint32(attr.Value)
	}
	return 0
}

func (p radiusPacket) octets(octetsAttr, gigawordsAttr uint8) int64 {
	var octets uint32
	var gigawords uint32
	for _, attr := range p.Attributes {
		if len(attr.Value) != 4 {
			continue
		}
		switch attr.Type {
		case octetsAttr:
			octets = binary.BigEndian.Uint32(attr.Value)
		case gigawordsAttr:
			gigawords = binary.BigEndian.Uint32(attr.Value)
		}
	}
	return (int64(gigawords) << 32) + int64(octets)
}

func radiusUint32Attr(attrType uint8, value uint32) radiusAttr {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, value)
	return radiusAttr{Type: attrType, Value: buf}
}

func radiusMikrotikRateLimitAttr(rate string) radiusAttr {
	// MikroTik VSA: vendor 14988, type 8, value "<tx>/<rx>".
	payload := make([]byte, 0, 6+len(rate))
	vendor := make([]byte, 4)
	binary.BigEndian.PutUint32(vendor, 14988)
	payload = append(payload, vendor...)
	payload = append(payload, 8, uint8(2+len(rate)))
	payload = append(payload, []byte(rate)...)
	return radiusAttr{Type: radiusAttrVendorSpecific, Value: payload}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
