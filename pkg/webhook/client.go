package webhook

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
)

// WebhookClient implements bridgev2.NetworkAPI for a single user login.
// Since this is a one-way bridge (webhook → Matrix), most methods are no-ops.
type WebhookClient struct {
	Connector *WebhookConnector
	UserLogin *bridgev2.UserLogin
}

var _ bridgev2.NetworkAPI = (*WebhookClient)(nil)

func (c *WebhookClient) Connect(ctx context.Context) {
	c.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateConnected,
	})
}

func (c *WebhookClient) Disconnect() {}

func (c *WebhookClient) IsLoggedIn() bool {
	return true
}

func (c *WebhookClient) LogoutRemote(ctx context.Context) {
	c.Connector.setLogin(nil)
}

func (c *WebhookClient) IsThisUser(_ context.Context, userID networkid.UserID) bool {
	return userID == "webhook"
}

func (c *WebhookClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	roomKey := string(portal.ID)
	roomName := c.Connector.getRoomName(roomKey)
	if roomName == "" {
		roomName = roomKey
	}

	inviteUsers := make([]bridgev2.ChatMember, len(c.Connector.Config.InviteUsers))
	for i, u := range c.Connector.Config.InviteUsers {
		inviteUsers[i] = bridgev2.ChatMember{
			EventSender: bridgev2.EventSender{
				Sender: networkid.UserID(u),
			},
		}
	}

	return &bridgev2.ChatInfo{
		Name:  &roomName,
		Topic: strPtr("Webhook notifications: " + roomKey),
		ExtraUpdates: func(ctx context.Context, p *bridgev2.Portal) bool {
			meta := p.Metadata.(*PortalMetadata)
			if meta.RoomName != roomName {
				meta.RoomName = roomName
				return true
			}
			return false
		},
	}, nil
}

func (c *WebhookClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	// Ghost ID is "webhook-<name>", use the webhook name for display
	name := string(ghost.ID)
	if len(name) > 8 && name[:8] == "webhook-" {
		// Look up sender_name from webhook config
		whName := name[8:]
		for _, wh := range c.Connector.Config.Webhooks {
			if wh.Name == whName {
				senderName := wh.SenderName
				if senderName == "" {
					senderName = wh.Name
				}
				name = senderName
				break
			}
		}
	}

	return &bridgev2.UserInfo{
		Name: &name,
	}, nil
}

func (c *WebhookClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	// One-way bridge: webhook → Matrix only. Ignore Matrix messages.
	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:       networkid.MessageID("ignored"),
			SenderID: "webhook",
		},
	}, nil
}

func (c *WebhookClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	return &event.RoomFeatures{}
}

func strPtr(s string) *string {
	return &s
}

// WebhookLogin implements the login flow. Since there are no credentials to
// provide (the bridge authenticates via appservice tokens), login completes
// immediately.
type WebhookLogin struct {
	User      *bridgev2.User
	Connector *WebhookConnector
}

var _ bridgev2.LoginProcess = (*WebhookLogin)(nil)

func (l *WebhookLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	ul, err := l.User.NewLogin(ctx, &database.UserLogin{
		ID:         "webhook",
		RemoteName: "Webhook Bridge",
		Metadata:   &UserLoginMetadata{},
	}, &bridgev2.NewLoginParams{
		DeleteOnConflict: true,
	})
	if err != nil {
		return nil, err
	}

	l.Connector.setLogin(ul)
	go ul.Client.Connect(l.Connector.Bridge.BackgroundCtx)

	return &bridgev2.LoginStep{
		Type:   bridgev2.LoginStepTypeComplete,
		StepID: "complete",
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: ul.ID,
			UserLogin:   ul,
		},
	}, nil
}

func (l *WebhookLogin) Cancel() {}
