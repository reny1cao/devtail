package vm

import (
	"bytes"
	"fmt"
	"text/template"
)

const cloudInitTemplate = `#cloud-config
hostname: devtail-{{.VMID}}

users:
  - name: devtail
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - {{.SSHPublicKey}}

package_update: true
package_upgrade: true

packages:
  - curl
  - git
  - tmux
  - python3-pip
  - nodejs
  - npm
  - build-essential
  - tmate
  - jq

write_files:
  - path: /etc/systemd/system/gateway.service
    content: |
      [Unit]
      Description=DevTail Gateway
      After=network.target tailscaled.service

      [Service]
      Type=simple
      User=devtail
      WorkingDirectory=/home/devtail/workspace
      ExecStart=/usr/local/bin/gateway --port 8080 --workdir /home/devtail/workspace
      Restart=always
      RestartSec=10
      Environment="PATH=/usr/local/bin:/usr/bin:/bin:/home/devtail/.local/bin"

      [Install]
      WantedBy=multi-user.target

  - path: /home/devtail/.config/aider/config.yml
    content: |
      model: claude-3-sonnet-20240229
      edit-format: diff
      stream: true
      auto-commits: false
    owner: devtail:devtail

runcmd:
  # Install Tailscale
  - curl -fsSL https://tailscale.com/install.sh | sh
  - tailscale up --authkey={{.TailscaleAuthKey}} --ssh --hostname=devtail-{{.VMID}}
  
  # Install gateway binary
  - |
    curl -fsSL https://github.com/devtail/gateway/releases/latest/download/gateway-linux-amd64 \
      -o /usr/local/bin/gateway || \
    curl -fsSL {{.GatewayURL}} -o /usr/local/bin/gateway
  - chmod +x /usr/local/bin/gateway
  
  # Install aider
  - sudo -u devtail pip3 install --user aider-chat
  
  # Install openvscode-server
  - |
    sudo -u devtail bash -c "
      curl -fsSL https://github.com/gitpod-io/openvscode-server/releases/download/openvscode-server-v1.84.2/openvscode-server-v1.84.2-linux-x64.tar.gz | \
      tar -xz -C /home/devtail
      mv /home/devtail/openvscode-server-* /home/devtail/openvscode-server
    "
  
  # Create workspace directory
  - mkdir -p /home/devtail/workspace
  - chown -R devtail:devtail /home/devtail
  
  # Enable and start gateway
  - systemctl daemon-reload
  - systemctl enable gateway
  - systemctl start gateway
  
  # Send ready signal
  - |
    TAILSCALE_IP=$(tailscale ip -4)
    curl -X POST {{.CallbackURL}} \
      -H "Content-Type: application/json" \
      -d "{\"vm_id\":\"{{.VMID}}\",\"tailscale_ip\":\"$TAILSCALE_IP\",\"status\":\"ready\"}" || true

final_message: "DevTail VM ready in $UPTIME seconds"
`

type CloudInitData struct {
	VMID             string
	TailscaleAuthKey string
	SSHPublicKey     string
	GatewayURL       string
	CallbackURL      string
}

func GenerateCloudInit(data CloudInitData) (string, error) {
	tmpl, err := template.New("cloudinit").Parse(cloudInitTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}