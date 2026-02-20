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
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-simplex/pkg/simplexclient"
	"go.mau.fi/mautrix-simplex/pkg/simplexid"
)

// SimplexClient implements bridgev2.NetworkAPI.
type SimplexClient struct {
	Main      *SimplexConnector
	UserLogin *bridgev2.UserLogin
	Client    *simplexclient.Client

	wsURL    string
	stopCh   chan struct{}
	cancelFn context.CancelFunc
}

var _ bridgev2.NetworkAPI = (*SimplexClient)(nil)

func (s *SimplexClient) Connect(ctx context.Context) {
	meta := s.UserLogin.Metadata.(*simplexid.UserLoginMetadata)
	if s.wsURL == "" && meta.WSUrl == "" {
		s.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Message:    "No WebSocket URL configured. Please log in again.",
		})
		return
	}
	if s.wsURL == "" {
		s.wsURL = meta.WSUrl
	}
	s.tryConnect(ctx, 0)
}

func (s *SimplexClient) tryConnect(ctx context.Context, retryCount int) {
	if retryCount == 0 {
		s.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting})
	}

	log := zerolog.Ctx(ctx)
	client, err := simplexclient.New(ctx, s.wsURL, zerolog.Ctx(ctx).With().Str("component", "simplexclient").Logger())
	if err != nil {
		log.Err(err).Msg("Failed to connect to simplex-chat WebSocket")
		s.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      "websocket-connect-error",
			Message:    err.Error(),
		})
		retryIn := 2 << retryCount
		if retryIn > 150 {
			retryIn = 150
		}
		log.Debug().Int("retry_in_seconds", retryIn).Msg("Retrying connection")
		select {
		case <-time.After(time.Duration(retryIn) * time.Second):
		case <-ctx.Done():
			return
		}
		s.tryConnect(ctx, retryCount+1)
		return
	}

	s.Client = client
	s.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})
	log.Info().Str("ws_url", s.wsURL).Msg("Connected to simplex-chat")

	// Sync contacts and groups on every connect to keep avatars/profiles up to date
	go s.syncChats(ctx)

	// Start event loop
	connCtx, cancel := context.WithCancel(ctx)
	s.cancelFn = cancel
	go s.eventLoop(connCtx)
}

func (s *SimplexClient) eventLoop(ctx context.Context) {
	log := zerolog.Ctx(ctx)
	events := s.Client.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				log.Info().Msg("SimpleX event channel closed, reconnecting")
				s.UserLogin.BridgeState.Send(status.BridgeState{
					StateEvent: status.StateTransientDisconnect,
					Error:      "websocket-closed",
					Message:    "WebSocket connection closed",
				})
				go s.tryConnect(ctx, 0)
				return
			}
			s.handleSimplexEvent(ctx, evt)
		}
	}
}

func (s *SimplexClient) Disconnect() {
	if s.cancelFn != nil {
		s.cancelFn()
	}
	if s.Client != nil {
		if err := s.Client.Close(); err != nil {
			// Ignore close errors during disconnect
			_ = err
		}
		s.Client = nil
	}
}

func (s *SimplexClient) IsLoggedIn() bool {
	return s.Client != nil
}

func (s *SimplexClient) LogoutRemote(ctx context.Context) {
	s.Disconnect()
}

func (s *SimplexClient) IsThisUser(_ context.Context, userID networkid.UserID) bool {
	if s.Client == nil {
		return false
	}
	loginUserID, err := simplexid.ParseUserLoginID(s.UserLogin.ID)
	if err != nil {
		return false
	}
	return userID == simplexid.MakeUserID(loginUserID)
}

// makePortalKey creates a portal key for a given chat item.
func (s *SimplexClient) makePortalKeyFromChatInfo(chatInfo simplexclient.ChatInfo) networkid.PortalKey {
	var portalID networkid.PortalID
	if chatInfo.Type == "direct" && chatInfo.Contact != nil {
		portalID = simplexid.MakeDMPortalID(chatInfo.Contact.ContactID)
	} else if chatInfo.Type == "group" && chatInfo.GroupInfo != nil {
		portalID = simplexid.MakeGroupPortalID(chatInfo.GroupInfo.GroupID)
	} else {
		portalID = networkid.PortalID(fmt.Sprintf("unknown:%s", chatInfo.Type))
	}
	return networkid.PortalKey{
		ID:       portalID,
		Receiver: s.UserLogin.ID,
	}
}

// makeEventSender creates an EventSender for a chat item direction.
func (s *SimplexClient) makeEventSenderFromDir(dir simplexclient.ChatItemDir) bridgev2.EventSender {
	switch dir.Type {
	case "directSnd", "groupSnd":
		// Sent by us
		loginID, _ := simplexid.ParseUserLoginID(s.UserLogin.ID)
		return bridgev2.EventSender{
			IsFromMe: true,
			Sender:   simplexid.MakeUserID(loginID),
		}
	case "directRcv":
		// We need contact ID — it's not in dir, so use placeholder
		return bridgev2.EventSender{
			Sender: "unknown",
		}
	case "groupRcv":
		if dir.GroupMember != nil {
			var userID networkid.UserID
			if dir.GroupMember.ContactID != nil {
				userID = simplexid.MakeUserID(*dir.GroupMember.ContactID)
			} else {
				userID = simplexid.MakeMemberUserID(dir.GroupMember.MemberID)
			}
			return bridgev2.EventSender{
				Sender: userID,
			}
		}
		return bridgev2.EventSender{Sender: "unknown"}
	default:
		return bridgev2.EventSender{Sender: "unknown"}
	}
}

// makeEventSenderFromContact creates an EventSender from a contact.
func (s *SimplexClient) makeEventSenderFromContact(contact *simplexclient.Contact) bridgev2.EventSender {
	if contact == nil {
		return bridgev2.EventSender{Sender: "unknown"}
	}
	return bridgev2.EventSender{
		Sender: simplexid.MakeUserID(contact.ContactID),
	}
}

// makeEventSenderFromMember creates an EventSender from a group member.
func (s *SimplexClient) makeEventSenderFromMember(member *simplexclient.GroupMember) bridgev2.EventSender {
	if member == nil {
		return bridgev2.EventSender{Sender: "unknown"}
	}
	var userID networkid.UserID
	if member.ContactID != nil {
		userID = simplexid.MakeUserID(*member.ContactID)
	} else {
		userID = simplexid.MakeMemberUserID(member.MemberID)
	}
	return bridgev2.EventSender{Sender: userID}
}
