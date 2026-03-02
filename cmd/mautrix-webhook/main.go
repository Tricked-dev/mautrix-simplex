package main

import (
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	"go.mau.fi/mautrix-simplex/pkg/webhook"
)

var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var m = mxmain.BridgeMain{
	Name:        "mautrix-webhook",
	URL:         "https://github.com/tricked-dev/mautrix-simplex",
	Description: "A config-driven webhook-to-Matrix bridge.",
	Version:     "0.1.0",
	Connector:   &webhook.WebhookConnector{},
}

func main() {
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}
