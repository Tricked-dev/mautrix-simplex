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
	"os/exec"
	"strconv"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-simplex/pkg/simplexclient"
	"go.mau.fi/mautrix-simplex/pkg/simplexid"
)

// --- WebSocketLogin ---

// WebSocketLogin handles login by connecting to an existing simplex-chat process.
type WebSocketLogin struct {
	User *bridgev2.User
	Main *SimplexConnector
}

var _ bridgev2.LoginProcessUserInput = (*WebSocketLogin)(nil)

const (
	LoginStepWSURL    = "fi.mau.simplex.login.ws_url"
	LoginStepComplete = "fi.mau.simplex.login.complete"
)

func (w *WebSocketLogin) Cancel() {}

func (w *WebSocketLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       LoginStepWSURL,
		Instructions: "Enter the WebSocket URL of your running simplex-chat instance (e.g. ws://localhost:5225)",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					Type:    bridgev2.LoginInputFieldTypeURL,
					ID:      "ws_url",
					Name:    "WebSocket URL",
					Pattern: `^wss?://.+`,
				},
			},
		},
	}, nil
}

func (w *WebSocketLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	wsURL, ok := input["ws_url"]
	if !ok || wsURL == "" {
		return nil, fmt.Errorf("ws_url is required")
	}

	log := zerolog.Ctx(ctx)
	log.Info().Str("ws_url", wsURL).Msg("Connecting to simplex-chat to verify login")

	// Connect to the simplex-chat instance to get the active user
	client, err := simplexclient.New(ctx, wsURL, log.With().Str("component", "simplexclient").Logger())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to simplex-chat: %w", err)
	}
	defer client.Close()

	user, err := client.GetActiveUser()
	if err != nil {
		return nil, fmt.Errorf("failed to get active user: %w", err)
	}

	loginID := simplexid.MakeUserLoginID(user.UserID)
	ul, err := w.User.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: user.Profile.DisplayName,
		RemoteProfile: status.RemoteProfile{
			Name: user.Profile.DisplayName,
		},
		Metadata: &simplexid.UserLoginMetadata{
			WSUrl: wsURL,
		},
	}, &bridgev2.NewLoginParams{
		DeleteOnConflict: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create user login: %w", err)
	}

	// Kick off connection
	go ul.Client.(*SimplexClient).Connect(w.Main.Bridge.BackgroundCtx)

	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       LoginStepComplete,
		Instructions: fmt.Sprintf("Successfully logged in as %s (user ID %d)", user.Profile.DisplayName, user.UserID),
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: ul.ID,
			UserLogin:   ul,
		},
	}, nil
}

// --- ManagedLogin ---

// ManagedLogin handles login by having the bridge manage the simplex-chat process.
type ManagedLogin struct {
	User *bridgev2.User
	Main *SimplexConnector
}

var _ bridgev2.LoginProcessUserInput = (*ManagedLogin)(nil)

const (
	LoginStepManagedDBPath = "fi.mau.simplex.login.managed_db_path"
)

func (m *ManagedLogin) Cancel() {}

func (m *ManagedLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       LoginStepManagedDBPath,
		Instructions: "Enter the path to your SimpleX Chat database directory (the directory containing your profile files)",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					Type: bridgev2.LoginInputFieldTypeToken,
					ID:   "db_path",
					Name: "Database path",
				},
			},
		},
	}, nil
}

func (m *ManagedLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	dbPath, ok := input["db_path"]
	if !ok || dbPath == "" {
		return nil, fmt.Errorf("db_path is required")
	}

	log := zerolog.Ctx(ctx)

	// Find a free port
	port, err := findFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to find free port: %w", err)
	}
	wsURL := fmt.Sprintf("ws://localhost:%d", port)

	log.Info().Str("db_path", dbPath).Int("port", port).Msg("Starting managed simplex-chat process")

	// Start simplex-chat
	cmd := exec.CommandContext(ctx, "simplex-chat", "-p", strconv.Itoa(port), "-d", dbPath)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start simplex-chat: %w", err)
	}

	// Give it a moment to start up, then connect
	// (A real implementation would poll with retries)
	simplexLog := log.With().Str("component", "simplexclient").Logger()
	var client *simplexclient.Client
	for attempts := 0; attempts < 10; attempts++ {
		client, err = simplexclient.New(ctx, wsURL, simplexLog)
		if err == nil {
			break
		}
		log.Debug().Err(err).Int("attempt", attempts+1).Msg("Waiting for simplex-chat to start")
	}
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("simplex-chat failed to become ready: %w", err)
	}
	defer client.Close()

	user, err := client.GetActiveUser()
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("failed to get active user: %w", err)
	}

	loginID := simplexid.MakeUserLoginID(user.UserID)
	ul, err := m.User.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: user.Profile.DisplayName,
		RemoteProfile: status.RemoteProfile{
			Name: user.Profile.DisplayName,
		},
		Metadata: &simplexid.UserLoginMetadata{
			WSUrl:   wsURL,
			DBPath:  dbPath,
			Managed: true,
		},
	}, &bridgev2.NewLoginParams{
		DeleteOnConflict: true,
	})
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("failed to create user login: %w", err)
	}

	// The actual managed process lifecycle will be handled during Connect()
	go ul.Client.(*SimplexClient).Connect(m.Main.Bridge.BackgroundCtx)

	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       LoginStepComplete,
		Instructions: fmt.Sprintf("Successfully started managed simplex-chat for %s (user ID %d)", user.Profile.DisplayName, user.UserID),
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: ul.ID,
			UserLogin:   ul,
		},
	}, nil
}

// findFreePort finds an available TCP port.
func findFreePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
