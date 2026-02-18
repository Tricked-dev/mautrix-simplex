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

package simplexclient

import "encoding/json"

// ChatType represents the type of chat (Direct or Group)
type ChatType string

const (
	ChatTypeDirect ChatType = "@"
	ChatTypeGroup  ChatType = "#"
)

// ChatRef is a reference to a chat
type ChatRef struct {
	ChatType ChatType `json:"chatType"`
	ChatID   int64    `json:"chatId"`
}

// ChatPaginationType represents the pagination type
type ChatPaginationType string

const (
	PaginationLast    ChatPaginationType = "last"
	PaginationBefore  ChatPaginationType = "before"
	PaginationAfter   ChatPaginationType = "after"
	PaginationAround  ChatPaginationType = "around"
	PaginationInitial ChatPaginationType = "initial"
)

// ChatPagination for APIGetChat requests
type ChatPagination struct {
	Type   ChatPaginationType `json:"type"`
	ItemID int64              `json:"chatItemId,omitempty"`
	Count  int                `json:"count"`
}

// DeleteMode for APIDeleteChatItem
type DeleteMode string

const (
	DeleteModeBroadcast DeleteMode = "broadcast"
	DeleteModeInternal  DeleteMode = "internal"
)

// GroupMemberRole for group members
type GroupMemberRole string

const (
	GroupMemberRoleMember    GroupMemberRole = "member"
	GroupMemberRoleAdmin     GroupMemberRole = "admin"
	GroupMemberRoleOwner     GroupMemberRole = "owner"
	GroupMemberRoleModerator GroupMemberRole = "moderator"
	GroupMemberRoleAuthor    GroupMemberRole = "author"
	GroupMemberRoleObserver  GroupMemberRole = "observer"
)

// User represents a local user
type User struct {
	UserID  int64   `json:"userId"`
	Profile Profile `json:"profile"`
}

// Profile represents a user or contact profile
type Profile struct {
	DisplayName string  `json:"displayName"`
	FullName    string  `json:"fullName"`
	Image       *string `json:"image,omitempty"`
}

// GroupProfile represents a group profile
type GroupProfile struct {
	DisplayName string  `json:"displayName"`
	FullName    string  `json:"fullName"`
	Image       *string `json:"image,omitempty"`
	Description *string `json:"groupDescription,omitempty"`
}

// Contact represents a SimpleX contact
type Contact struct {
	ContactID        int64   `json:"contactId"`
	LocalDisplayName string  `json:"localDisplayName"`
	Profile          Profile `json:"profile"`
	ContactUsed      bool    `json:"contactUsed"`
	CreatedAt        string  `json:"createdAt"`
}

// GroupInfo represents a group
type GroupInfo struct {
	GroupID          int64        `json:"groupId"`
	LocalDisplayName string       `json:"localDisplayName"`
	GroupProfile     GroupProfile `json:"groupProfile"`
	Membership       GroupMember  `json:"membership"`
	CreatedAt        string       `json:"createdAt"`
}

// GroupMember represents a group member
type GroupMember struct {
	GroupMemberID    int64           `json:"groupMemberId"`
	GroupID          int64           `json:"groupId"`
	MemberID         string          `json:"memberId"` // base64-encoded
	MemberRole       GroupMemberRole `json:"memberRole"`
	MemberCategory   string          `json:"memberCategory"`
	MemberStatus     string          `json:"memberStatus"`
	LocalDisplayName string          `json:"localDisplayName"`
	Profile          Profile         `json:"profile"`
	ContactID        *int64          `json:"contactId,omitempty"`
}

// ChatItemMeta contains metadata about a chat item
type ChatItemMeta struct {
	ItemID      int64           `json:"itemId"`
	ItemSent    bool            `json:"itemSent"`
	CreatedAt   string          `json:"createdAt"`
	ItemTimed   *ItemTimedData  `json:"itemTimed,omitempty"`
	ItemText    string          `json:"itemText"`
	ItemStatus  json.RawMessage `json:"itemStatus"`
	ItemDeleted *ItemDeleted    `json:"itemDeleted,omitempty"`
	ItemEdited  bool            `json:"itemEdited,omitempty"`
	ItemLive    *bool           `json:"itemLive,omitempty"`
}

// ItemTimedData contains timed message data
type ItemTimedData struct {
	TTL      int     `json:"ttl"`
	DeleteAt *string `json:"deleteAt,omitempty"`
}

// ItemDeleted contains deletion info
type ItemDeleted struct {
	Type    string  `json:"type"`
	ByGroup *Member `json:"byGroupMember,omitempty"`
}

// Member represents a minimal member reference
type Member struct {
	GroupMemberID int64 `json:"groupMemberId"`
}

// ChatItemContent represents the content of a message
type ChatItemContent struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	File       *FileTransfer   `json:"file,omitempty"`
	MsgContent json.RawMessage `json:"msgContent,omitempty"`
}

// MsgContent represents message content for sending.
// For "text": Type="text", Text=body
// For "image": Type="image", Text=alt/filename, Image=base64 thumbnail (required, may be empty string "")
// For "file": Type="file", Text=filename
// For "video": Type="video", Text=filename, Image=thumbnail (required), Duration=seconds (required)
// For "voice": Type="voice", Text="", Duration=seconds (required)
// Note: filePath is NOT a valid field here; file path goes in ComposedMessage.FileSource.
// Note: "image" field is required (not omitempty) for MCImage/MCVideo; "duration" is required for MCVideo/MCVoice.
// Use MakeMsgContent helpers to construct correctly.
type MsgContent struct {
	Type     string  `json:"type"`
	Text     string  `json:"text,omitempty"`
	Image    *string `json:"image,omitempty"`    // base64 thumbnail for image/video; required for MCImage/MCVideo
	Duration *int    `json:"duration,omitempty"` // seconds for video/voice; required for MCVideo/MCVoice
}

// MakeMsgContentText returns a text MsgContent.
func MakeMsgContentText(text string) MsgContent {
	return MsgContent{Type: "text", Text: text}
}

// MakeMsgContentFile returns a file MsgContent.
func MakeMsgContentFile(filename string) MsgContent {
	return MsgContent{Type: "file", Text: filename}
}

// MakeMsgContentImage returns an image MsgContent. Image thumbnail may be empty string.
func MakeMsgContentImage(text, imageThumbnail string) MsgContent {
	return MsgContent{Type: "image", Text: text, Image: &imageThumbnail}
}

// MakeMsgContentVideo returns a video MsgContent. Image may be empty; duration in seconds.
func MakeMsgContentVideo(text, imageThumbnail string, duration int) MsgContent {
	return MsgContent{Type: "video", Text: text, Image: &imageThumbnail, Duration: &duration}
}

// MakeMsgContentVoice returns a voice MsgContent. Duration in seconds.
func MakeMsgContentVoice(text string, duration int) MsgContent {
	return MsgContent{Type: "voice", Text: text, Duration: &duration}
}

// FileTransfer represents a file transfer
type FileTransfer struct {
	FileID     int64           `json:"fileId"`
	FileName   string          `json:"fileName"`
	FileSize   int64           `json:"fileSize"`
	FilePath   *string         `json:"filePath,omitempty"`
	FileStatus json.RawMessage `json:"fileStatus"`
}

// CIFile represents a file attached to a chat item
type CIFile struct {
	FileID     int64           `json:"fileId"`
	FileName   string          `json:"fileName"`
	FileSize   int64           `json:"fileSize"`
	FilePath   *string         `json:"filePath,omitempty"`
	FileStatus json.RawMessage `json:"fileStatus"`
}

// ChatItem represents a message
type ChatItem struct {
	ChatDir       ChatItemDir                     `json:"chatDir"`
	Meta          ChatItemMeta                    `json:"meta"`
	Content       ChatItemContent                 `json:"content"`
	FormattedText []FormattedText                 `json:"formattedText,omitempty"`
	File          *CIFile                         `json:"file,omitempty"`
	Reactions     []CIReactionCount               `json:"reactions,omitempty"`
	QuotedItem    *CIQuote                        `json:"quotedItem,omitempty"`
	Mentions      map[string]CIGroupMemberMention `json:"mentions,omitempty"`
}

// ChatItemDir represents the direction of a chat item
type ChatItemDir struct {
	Type        string       `json:"type"` // "directSnd", "directRcv", "groupSnd", "groupRcv"
	GroupMember *GroupMember `json:"groupMember,omitempty"`
}

// FormattedText represents formatted text spans
type FormattedText struct {
	Text   string  `json:"text"`
	Format *Format `json:"format,omitempty"`
}

// Format represents text formatting
type Format struct {
	Type string `json:"type"` // "bold", "italic", "strikeThrough", "snipped", "colored", "uri", "email", "phone", "mention"
}

// CIReactionCount represents an emoji reaction with count
type CIReactionCount struct {
	Reaction      MsgReaction `json:"reaction"`
	ReactionCount int         `json:"reactionCount"`
}

// MsgReaction represents a reaction
type MsgReaction struct {
	Type  string `json:"type"` // "emoji"
	Emoji string `json:"emoji,omitempty"`
}

// CIQuote represents a quoted item
type CIQuote struct {
	ChatDir       *ChatItemDir    `json:"chatDir,omitempty"`
	ItemID        *int64          `json:"itemId,omitempty"`
	SharedMsgID   *string         `json:"sharedMsgId,omitempty"`
	SentAt        string          `json:"sentAt"`
	Content       ChatItemContent `json:"content"`
	FormattedText []FormattedText `json:"formattedText,omitempty"`
}

// CIGroupMemberMention represents a mention in a group message
type CIGroupMemberMention struct {
	MemberID         string `json:"memberId"`
	GroupMemberID    int64  `json:"groupMemberId"`
	LocalDisplayName string `json:"localDisplayName"`
}

// AChatItem wraps a chat item with its chat info
type AChatItem struct {
	ChatInfo ChatInfo `json:"chatInfo"`
	ChatItem ChatItem `json:"chatItem"`
}

// ChatInfo contains info about a chat
type ChatInfo struct {
	Type      string     `json:"type"` // "direct", "group"
	Contact   *Contact   `json:"contact,omitempty"`
	GroupInfo *GroupInfo `json:"groupInfo,omitempty"`
}

// AChat wraps chat items with chat info
type AChat struct {
	ChatInfo  ChatInfo   `json:"chatInfo"`
	ChatItems []ChatItem `json:"chatItems"`
}

// ChatItemDeletion represents a deletion
type ChatItemDeletion struct {
	DeletedChatItem *AChatItem `json:"deletedChatItem,omitempty"`
	ToItem          *AChatItem `json:"toChatItem,omitempty"`
}

// CIReaction represents an emoji reaction event
type CIReaction struct {
	Reaction   MsgReaction `json:"reaction"`
	ReactionAt string      `json:"reactionAt"`
}

// ACIReaction wraps a reaction with chat info
type ACIReaction struct {
	ChatInfo     ChatInfo     `json:"chatInfo"`
	ChatReaction CIReaction   `json:"chatReaction"`
	Reaction     MsgReaction  `json:"reaction"`
	FromMember   *GroupMember `json:"fromMember,omitempty"`
	FromContact  *Contact     `json:"fromContact,omitempty"`
}

// UserContactRequest represents a contact request
type UserContactRequest struct {
	ContactRequestID int64   `json:"contactRequestId"`
	LocalDisplayName string  `json:"localDisplayName"`
	Profile          Profile `json:"profile"`
}

// ComposedMessage is a message to be sent
type ComposedMessage struct {
	FileSource   *CryptoFile      `json:"fileSource,omitempty"`
	QuotedItemID *int64           `json:"quotedItemId,omitempty"`
	Mentions     map[string]int64 `json:"mentions"`
	MsgContent   MsgContent       `json:"msgContent"`
}

// CryptoFile represents a file to be sent
type CryptoFile struct {
	FilePath   string      `json:"filePath"`
	CryptoArgs *CryptoArgs `json:"cryptoArgs,omitempty"`
}

// CryptoArgs contains optional encryption args
type CryptoArgs struct{}

// Event represents an async SimpleX event
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"-"`
	Raw  json.RawMessage `json:"-"`
}

// NewChatItemsEvent represents new messages event
type NewChatItemsEvent struct {
	User      User        `json:"user"`
	ChatItems []AChatItem `json:"chatItems"`
}

// ChatItemUpdatedEvent represents an edit event
type ChatItemUpdatedEvent struct {
	User     User      `json:"user"`
	ChatItem AChatItem `json:"chatItem"`
}

// ChatItemsDeletedEvent represents a deletion event
type ChatItemsDeletedEvent struct {
	User              User               `json:"user"`
	ChatItemDeletions []ChatItemDeletion `json:"chatItemDeletions"`
	ByUser            bool               `json:"byUser"`
}

// ChatItemReactionEvent represents a reaction event
type ChatItemReactionEvent struct {
	User     User        `json:"user"`
	Added    bool        `json:"added"`
	Reaction ACIReaction `json:"reaction"`
}

// ContactConnectedEvent represents a contact connected event
type ContactConnectedEvent struct {
	User    User    `json:"user"`
	Contact Contact `json:"contact"`
}

// ContactUpdatedEvent represents a contact update event
type ContactUpdatedEvent struct {
	User        User    `json:"user"`
	FromContact Contact `json:"fromContact"`
	ToContact   Contact `json:"toContact"`
}

// JoinedGroupMemberEvent represents a new member joining event
type JoinedGroupMemberEvent struct {
	User      User        `json:"user"`
	GroupInfo GroupInfo   `json:"groupInfo"`
	Member    GroupMember `json:"member"`
}

// DeletedMemberEvent represents a member being deleted event
type DeletedMemberEvent struct {
	User          User        `json:"user"`
	GroupInfo     GroupInfo   `json:"groupInfo"`
	ByMember      GroupMember `json:"byMember"`
	DeletedMember GroupMember `json:"deletedMember"`
}

// LeftMemberEvent represents a member leaving event
type LeftMemberEvent struct {
	User      User        `json:"user"`
	GroupInfo GroupInfo   `json:"groupInfo"`
	Member    GroupMember `json:"member"`
}

// GroupUpdatedEvent represents a group profile update
type GroupUpdatedEvent struct {
	User      User         `json:"user"`
	FromGroup GroupInfo    `json:"fromGroup"`
	ToGroup   GroupInfo    `json:"toGroup"`
	Member    *GroupMember `json:"member,omitempty"`
}

// ReceivedGroupInvitationEvent represents a group invitation
type ReceivedGroupInvitationEvent struct {
	User       User            `json:"user"`
	GroupInfo  GroupInfo       `json:"groupInfo"`
	Contact    Contact         `json:"contact"`
	MemberRole GroupMemberRole `json:"memberRole"`
}

// RcvFileCompleteEvent represents a completed file download
type RcvFileCompleteEvent struct {
	User     User      `json:"user"`
	ChatItem AChatItem `json:"chatItem"`
}

// ReceivedContactRequestEvent represents an incoming contact request
type ReceivedContactRequestEvent struct {
	User           User               `json:"user"`
	ContactRequest UserContactRequest `json:"contactRequest"`
}

// Response from SimpleX API
type Response struct {
	CorrID string          `json:"corrId"`
	Resp   json.RawMessage `json:"resp"`
	Type   string          `json:"-"`
}
