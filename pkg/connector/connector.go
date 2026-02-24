// mautrix-simplex - A Matrix-SimpleX puppeting bridge.
// Copyright (C) 2024 Tricked
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package connector

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-simplex/pkg/simplexid"
)

// SimplexConnector implements bridgev2.NetworkConnector.
type SimplexConnector struct {
	Bridge            *bridgev2.Bridge
	Config            SimplexConfig
	linkPreviewClient *http.Client
}

var _ bridgev2.NetworkConnector = (*SimplexConnector)(nil)

func (s *SimplexConnector) GetName() bridgev2.BridgeName {
	return bridgev2.BridgeName{
		DisplayName:      "SimpleX",
		NetworkURL:       "https://simplex.chat",
		NetworkIcon:      "mxc://maunium.net/simplex",
		NetworkID:        "simplex",
		BeeperBridgeType: "simplex",
		DefaultPort:      29340,
	}
}

func (s *SimplexConnector) Init(bridge *bridgev2.Bridge) {
	s.Bridge = bridge
}

func (s *SimplexConnector) Start(ctx context.Context) error {
	s.linkPreviewClient = makeLinkPreviewClient(s.Config.LinkPreviewFamilyDNS)
	return nil
}

// makeLinkPreviewClient returns an *http.Client for fetching link previews.
// If familyDNS is true, DNS resolution uses Cloudflare for Families servers
// (1.1.1.3 / 1.0.0.3 and their IPv6 equivalents) which filter malware and
// adult-content domains.
func makeLinkPreviewClient(familyDNS bool) *http.Client {
	if !familyDNS {
		return http.DefaultClient
	}
	// Cloudflare for Families nameservers — IPv4 primary/secondary then IPv6.
	nameservers := []string{
		"1.1.1.3:53",
		"1.0.0.3:53",
		"[2606:4700:4700::1113]:53",
		"[2606:4700:4700::1003]:53",
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := &net.Dialer{Timeout: 5 * time.Second}
			var lastErr error
			for _, ns := range nameservers {
				conn, err := d.DialContext(ctx, "udp", ns)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
	dialer := &net.Dialer{
		Timeout:  10 * time.Second,
		Resolver: resolver,
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
		},
	}
}

func (s *SimplexConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	meta := login.Metadata.(*simplexid.UserLoginMetadata)
	sc := &SimplexClient{
		Main:      s,
		UserLogin: login,
	}
	if meta.WSUrl != "" {
		sc.wsURL = meta.WSUrl
	}
	login.Client = sc
	return nil
}

func (s *SimplexConnector) GenerateTransactionID(userID id.UserID, roomID id.RoomID, eventType event.Type) networkid.RawTransactionID {
	return networkid.RawTransactionID(strconv.FormatInt(time.Now().UnixMilli(), 10))
}

func (s *SimplexConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return []bridgev2.LoginFlow{
		{
			Name:        "WebSocket URL",
			Description: "Connect to a running simplex-chat instance",
			ID:          "websocket",
		},
		{
			Name:        "Managed",
			Description: "Provide a SimpleX database path and let the bridge manage the process",
			ID:          "managed",
		},
	}
}

func (s *SimplexConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	switch flowID {
	case "websocket":
		return &WebSocketLogin{User: user, Main: s}, nil
	case "managed":
		return &ManagedLogin{User: user, Main: s}, nil
	default:
		return nil, fmt.Errorf("invalid login flow ID: %s", flowID)
	}
}
