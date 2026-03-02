package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"
	up "go.mau.fi/util/configupgrade"
)

type WebhookConnector struct {
	Bridge *bridgev2.Bridge
	Config WebhookNetworkConfig

	login   *bridgev2.UserLogin
	loginMu sync.Mutex

	// Pending room info for portal creation (room_key → room_name)
	roomNames   map[string]string
	roomNamesMu sync.Mutex

	templates map[string]*compiledTemplates
}

var _ bridgev2.NetworkConnector = (*WebhookConnector)(nil)

func (w *WebhookConnector) GetName() bridgev2.BridgeName {
	return bridgev2.BridgeName{
		DisplayName:      "Webhook",
		NetworkURL:       "https://github.com/tricked-dev/mautrix-simplex",
		NetworkIcon:      "",
		NetworkID:        "webhook",
		BeeperBridgeType: "webhook",
		DefaultPort:      9000,
	}
}

func (w *WebhookConnector) Init(bridge *bridgev2.Bridge) {
	w.Bridge = bridge
	w.roomNames = make(map[string]string)
	w.templates = make(map[string]*compiledTemplates)
}

func (w *WebhookConnector) Start(ctx context.Context) error {
	log := w.Bridge.Log.With().Str("component", "webhook-http").Logger()

	// Compile templates for each webhook
	for _, wh := range w.Config.Webhooks {
		ct, err := CompileTemplates(wh)
		if err != nil {
			return fmt.Errorf("compile templates for %s: %w", wh.Name, err)
		}
		w.templates[wh.Name] = ct
	}

	// Start HTTP server in background
	mux := http.NewServeMux()
	for _, wh := range w.Config.Webhooks {
		ct := w.templates[wh.Name]
		mux.HandleFunc(wh.Path, w.makeHandler(wh, ct, log))
		log.Info().Str("name", wh.Name).Str("path", wh.Path).Msg("registered webhook handler")
	}

	addr := w.Config.ListenAddress
	if addr == "" {
		addr = "127.0.0.1:9000"
	}

	go func() {
		log.Info().Str("address", addr).Msg("starting webhook HTTP server")
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Error().Err(err).Msg("webhook HTTP server error")
		}
	}()

	return nil
}

func (w *WebhookConnector) GetConfig() (string, any, up.Upgrader) {
	return ExampleConfig, &w.Config, up.SimpleUpgrader(upgradeConfig)
}

func (w *WebhookConnector) GetDBMetaTypes() database.MetaTypes {
	return database.MetaTypes{
		Portal:    func() any { return &PortalMetadata{} },
		Ghost:     func() any { return &GhostMetadata{} },
		Message:   func() any { return &MessageMetadata{} },
		Reaction:  nil,
		UserLogin: func() any { return &UserLoginMetadata{} },
	}
}

func (w *WebhookConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return &bridgev2.NetworkGeneralCapabilities{}
}

func (w *WebhookConnector) GetBridgeInfoVersion() (info, capabilities int) {
	return 1, 1
}

func (w *WebhookConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return []bridgev2.LoginFlow{{
		Name:        "Activate",
		Description: "Activate the webhook bridge",
		ID:          "activate",
	}}
}

func (w *WebhookConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	if flowID != "activate" {
		return nil, fmt.Errorf("unknown login flow: %s", flowID)
	}
	return &WebhookLogin{User: user, Connector: w}, nil
}

func (w *WebhookConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	wc := &WebhookClient{
		Connector: w,
		UserLogin: login,
	}
	login.Client = wc

	w.loginMu.Lock()
	w.login = login
	w.loginMu.Unlock()

	return nil
}

func (w *WebhookConnector) GetUserID(loginID networkid.UserLoginID) networkid.UserID {
	return networkid.UserID(loginID)
}

// setLogin stores the active user login for webhook forwarding.
func (w *WebhookConnector) setLogin(login *bridgev2.UserLogin) {
	w.loginMu.Lock()
	w.login = login
	w.loginMu.Unlock()
}

// getLogin returns the active user login, or nil if none.
func (w *WebhookConnector) getLogin() *bridgev2.UserLogin {
	w.loginMu.Lock()
	defer w.loginMu.Unlock()
	return w.login
}

// setRoomName stores a room name for portal creation.
func (w *WebhookConnector) setRoomName(roomKey, roomName string) {
	w.roomNamesMu.Lock()
	w.roomNames[roomKey] = roomName
	w.roomNamesMu.Unlock()
}

// getRoomName retrieves a stored room name.
func (w *WebhookConnector) getRoomName(roomKey string) string {
	w.roomNamesMu.Lock()
	defer w.roomNamesMu.Unlock()
	return w.roomNames[roomKey]
}

func (w *WebhookConnector) makeHandler(wh WebhookConfig, ct *compiledTemplates, log zerolog.Logger) http.HandlerFunc {
	return func(wr http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			wr.Write([]byte("ok"))
			return
		}
		if r.Method != http.MethodPost {
			http.Error(wr, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		login := w.getLogin()
		if login == nil {
			log.Warn().Str("webhook", wh.Name).Msg("received webhook but no login is active")
			http.Error(wr, "bridge not activated", http.StatusServiceUnavailable)
			return
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Error().Err(err).Str("webhook", wh.Name).Msg("failed to decode webhook payload")
			http.Error(wr, err.Error(), http.StatusBadRequest)
			return
		}

		// Write raw payload to debug directory if configured
		if w.Config.DebugDir != "" {
			if err := os.MkdirAll(w.Config.DebugDir, 0750); err == nil {
				raw, _ := json.MarshalIndent(payload, "", "  ")
				fname := filepath.Join(w.Config.DebugDir, time.Now().Format("2006-01-02T15-04-05.000")+"-"+wh.Name+".json")
				os.WriteFile(fname, raw, 0640)
			}
		}

		// Render room key to determine portal
		roomKey, err := renderTemplate(ct.roomKey, payload)
		if err != nil {
			log.Error().Err(err).Str("webhook", wh.Name).Msg("failed to render room_key template")
			http.Error(wr, "template error", http.StatusInternalServerError)
			return
		}

		// Render and store room name for portal creation
		roomName, err := renderTemplate(ct.roomName, payload)
		if err != nil {
			roomName = roomKey
		}
		w.setRoomName(roomKey, roomName)

		portalKey := networkid.PortalKey{
			ID: networkid.PortalID(roomKey),
		}

		senderID := networkid.UserID("webhook-" + wh.Name)
		msgID := networkid.MessageID(xid.New().String())

		// Queue remote event — bridgev2 handles portal creation, encryption, etc.
		login.QueueRemoteEvent(&simplevent.Message[map[string]any]{
			EventMeta: simplevent.EventMeta{
				Type:         bridgev2.RemoteEventMessage,
				PortalKey:    portalKey,
				CreatePortal: true,
				Sender: bridgev2.EventSender{
					Sender: senderID,
				},
				Timestamp: time.Now(),
			},
			Data: payload,
			ID:   msgID,
			ConvertMessageFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, data map[string]any) (*bridgev2.ConvertedMessage, error) {
				plain, err := renderTemplate(ct.plain, data)
				if err != nil {
					return nil, fmt.Errorf("render plain template: %w", err)
				}

				content := &event.MessageEventContent{
					MsgType: event.MsgText,
					Body:    plain,
				}

				if ct.html != nil {
					htmlBody, err := renderTemplate(ct.html, data)
					if err == nil && htmlBody != "" {
						content.Format = event.FormatHTML
						content.FormattedBody = htmlBody
					}
				}

				return &bridgev2.ConvertedMessage{
					Parts: []*bridgev2.ConvertedMessagePart{{
						ID:      "",
						Type:    event.EventMessage,
						Content: content,
					}},
				}, nil
			},
		})

		wr.Header().Set("Content-Type", "application/json")
		wr.Write([]byte(`{"status":"ok"}`))
	}
}
