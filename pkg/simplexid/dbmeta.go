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

package simplexid

import "go.mau.fi/util/jsontime"

// PortalMetadata stores extra data about a portal room.
type PortalMetadata struct {
	// LastSync tracks the last time the portal info was synced.
	LastSync jsontime.Unix `json:"last_sync,omitempty"`
}

// MessageMetadata stores extra data about a message.
type MessageMetadata struct {
	// HasFile indicates the message has a file attachment.
	HasFile bool `json:"has_file,omitempty"`
}

// UserLoginMetadata stores extra data about a user login.
type UserLoginMetadata struct {
	// WSUrl is the WebSocket URL of the simplex-chat process.
	WSUrl string `json:"ws_url,omitempty"`
	// DBPath is the database path (managed mode only).
	DBPath string `json:"db_path,omitempty"`
	// Managed indicates the bridge manages the simplex-chat process.
	Managed bool `json:"managed,omitempty"`
	// ChatsSynced indicates whether contacts/groups have been enumerated.
	ChatsSynced bool `json:"chats_synced,omitempty"`
}

// GhostMetadata stores extra data about a ghost user.
type GhostMetadata struct {
	// ProfileFetchedAt is when the ghost profile was last fetched.
	ProfileFetchedAt jsontime.UnixMilli `json:"profile_fetched_at,omitempty"`
}
