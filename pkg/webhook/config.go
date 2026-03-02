package webhook

import (
	up "go.mau.fi/util/configupgrade"
	"gopkg.in/yaml.v3"
)

// WebhookNetworkConfig is the network-specific config placed under the "network:" key
// in the bridge config. The framework handles homeserver, appservice, encryption, etc.
type WebhookNetworkConfig struct {
	ListenAddress string          `yaml:"listen_address"`
	InviteUsers   []string        `yaml:"invite_users"`
	DebugDir      string          `yaml:"debug_dir"`
	Webhooks      []WebhookConfig `yaml:"webhooks"`
}

type WebhookConfig struct {
	Name       string         `yaml:"name"`
	Path       string         `yaml:"path"`
	RoomKey    string         `yaml:"room_key"`
	RoomName   string         `yaml:"room_name"`
	SenderName string         `yaml:"sender_name"`
	Template   TemplateConfig `yaml:"template"`
}

type TemplateConfig struct {
	Plain     string `yaml:"plain"`
	HTML      string `yaml:"html"`
	PlainFile string `yaml:"plain_file"`
	HTMLFile  string `yaml:"html_file"`
}

// Database metadata types
type PortalMetadata struct {
	RoomName string `json:"room_name,omitempty"`
}

type GhostMetadata struct{}
type MessageMetadata struct{}
type UserLoginMetadata struct{}

var ExampleConfig = `
listen_address: 127.0.0.1:9000
invite_users:
  - "@user:example.com"
debug_dir: ""
webhooks:
  - name: example
    path: /
    room_key: "notifications"
    room_name: "Notifications"
    sender_name: "Webhook Bot"
    template:
      plain: "{{.message}}"
      html: "<b>{{.message}}</b>"
`

func (c *WebhookNetworkConfig) UnmarshalYAML(node *yaml.Node) error {
	type raw WebhookNetworkConfig
	return node.Decode((*raw)(c))
}

func upgradeConfig(helper up.Helper) {
	helper.Copy(up.Str, "listen_address")
	helper.Copy(up.List, "invite_users")
	helper.Copy(up.Str, "debug_dir")
	helper.Copy(up.List, "webhooks")
}
