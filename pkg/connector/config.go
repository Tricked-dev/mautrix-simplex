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
	_ "embed"
	"strings"
	"text/template"

	up "go.mau.fi/util/configupgrade"
	"gopkg.in/yaml.v3"
)

//go:embed example-config.yaml
var ExampleConfig string

// SimplexConfig holds bridge-specific configuration.
type SimplexConfig struct {
	// DisplaynameTemplate is the Go template for formatting ghost display names.
	DisplaynameTemplate string `yaml:"displayname_template"`
	// SimplexBinary is the path to the simplex-chat binary (for managed mode).
	SimplexBinary string `yaml:"simplex_binary"`

	displaynameTemplate *template.Template `yaml:"-"`
}

type umSimplexConfig SimplexConfig

func (c *SimplexConfig) UnmarshalYAML(node *yaml.Node) error {
	err := node.Decode((*umSimplexConfig)(c))
	if err != nil {
		return err
	}
	return c.PostProcess()
}

func (c *SimplexConfig) PostProcess() error {
	var err error
	c.displaynameTemplate, err = template.New("displayname").Parse(c.DisplaynameTemplate)
	return err
}

// DisplaynameParams contains fields for the displayname template.
type DisplaynameParams struct {
	DisplayName string
	ContactID   int64
}

// FormatDisplayname formats a display name using the configured template.
func (c *SimplexConfig) FormatDisplayname(displayName string, contactID int64) string {
	var buf strings.Builder
	err := c.displaynameTemplate.Execute(&buf, &DisplaynameParams{
		DisplayName: displayName,
		ContactID:   contactID,
	})
	if err != nil {
		panic(err)
	}
	return buf.String()
}

func upgradeConfig(helper up.Helper) {
	helper.Copy(up.Str, "displayname_template")
	helper.Copy(up.Str, "simplex_binary")
}

func (s *SimplexConnector) GetConfig() (string, any, up.Upgrader) {
	return ExampleConfig, &s.Config, up.SimpleUpgrader(upgradeConfig)
}
