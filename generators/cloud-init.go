package generators

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lxd "github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"

	"github.com/lxc/distrobuilder/image"
	"github.com/lxc/distrobuilder/shared"
)

// CloudInitGenerator represents the cloud-init generator.
type CloudInitGenerator struct{}

// RunLXC disables cloud-init.
func (g CloudInitGenerator) RunLXC(cacheDir, sourceDir string, img *image.LXCImage,
	defFile shared.DefinitionFile) error {
	// With OpenRC:
	// Remove all symlinks to /etc/init.d/cloud-{init-local,config,init,final} in /etc/runlevels/*
	filepath.Walk(filepath.Join(sourceDir, "/etc/runlevels"), func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		if lxd.StringInSlice(info.Name(), []string{"cloud-init-local", "cloud-config", "cloud-init", "cloud-final"}) {
			err := os.Remove(path)
			if err != nil {
				return err
			}
		}

		return nil
	})

	// With systemd:
	// Create file /etc/cloud/cloud-init.disabled
	err := os.MkdirAll(filepath.Join(sourceDir, "/etc/cloud"), 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(sourceDir, "/etc/cloud/cloud-init.disabled"))
	if err != nil {
		return err
	}
	defer f.Close()

	return nil
}

// RunLXD creates cloud-init template files.
func (g CloudInitGenerator) RunLXD(cacheDir, sourceDir string, img *image.LXDImage,
	defFile shared.DefinitionFile) error {
	templateDir := filepath.Join(cacheDir, "templates")

	err := os.MkdirAll(templateDir, 0755)
	if err != nil {
		return err
	}

	var content string
	properties := make(map[string]string)

	switch defFile.Name {
	case "user-data":
		content = `{{ config_get("user.user-data", properties.default) }}
`
		properties["default"] = `#cloud-config
{}`
	case "meta-data":
		content = `instance-id: {{ container.name }}
local-hostname: {{ container.name }}
{{ config_get("user.meta-data", "") }}
`
	case "vendor-data":
		content = `{{ config_get("user.vendor-data", properties.default) }}
`
		properties["default"] = `#cloud-config
{}`
	case "network-config":
		content = `{% if config_get("user.network-config", "") == "" %}version: 1
config:
  - type: physical
    name: eth0
    subnets:
      - type: {% if config_get("user.network_mode", "") == "link-local" %}manual{% else %}dhcp{% endif %}
        control: auto{% else %}{{ config_get("user.network-config", "") }}{% endif %}
`
	default:
		return fmt.Errorf("Unknown cloud-init configuration: %s", defFile.Name)
	}

	template := fmt.Sprintf("cloud-init-%s.tpl", defFile.Name)

	file, err := os.Create(filepath.Join(templateDir, template))
	if err != nil {
		return err
	}

	defer file.Close()

	if defFile.Content != "" {
		content = defFile.Content
	}

	// Append final new line if missing
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	_, err = file.WriteString(content)
	if err != nil {
		return fmt.Errorf("Failed to write to content to %s template: %s", defFile.Name, err)
	}

	if len(defFile.Template.Properties) > 0 {
		properties = defFile.Template.Properties
	}

	targetPath := filepath.Join("/var/lib/cloud/seed/nocloud-net", defFile.Name)

	if defFile.Path != "" {
		targetPath = defFile.Path
	}

	// Add to LXD templates
	img.Metadata.Templates[targetPath] = &api.ImageMetadataTemplate{
		Template:   template,
		Properties: properties,
		When:       []string{"create", "copy"},
	}

	return err
}

// Run does nothing.
func (g CloudInitGenerator) Run(cacheDir, sourceDir string,
	defFile shared.DefinitionFile) error {
	return nil
}
