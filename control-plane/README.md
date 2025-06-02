# DevTail Control Plane

The control plane manages VM lifecycle, provisioning Hetzner instances with Tailscale networking.

## Architecture

```
Mobile App -> Control Plane API -> Hetzner Cloud
                                -> Tailscale API
                                -> PostgreSQL
```

## API Endpoints

### Create VM
```bash
POST /api/v1/vms
X-User-ID: user123

{
  "spec": {
    "type": "cx11",
    "location": "nbg1",
    "disk_size": 40
  }
}

Response:
{
  "vm": {
    "id": "vm-uuid",
    "status": "provisioning",
    "websocket_token": "..."
  },
  "websocket_url": "wss://gateway.devtail.com/ws?token=...",
  "estimated_ready_seconds": 60
}
```

### Get VM Status
```bash
GET /api/v1/vms/{vm-id}
X-User-ID: user123
```

### Delete VM
```bash
DELETE /api/v1/vms/{vm-id}
X-User-ID: user123
```

## Configuration

Copy `config.example.yaml` to `config.yaml` and fill in:

1. **Hetzner API Token**: From https://console.hetzner.cloud/
2. **Tailscale API Key**: From https://login.tailscale.com/admin/settings/keys
3. **SSH Key**: Upload to Hetzner and note the ID
4. **Database**: PostgreSQL connection string

## Deployment

```bash
# Setup database
createdb devtail
make migrate

# Run locally
make run

# Or with Docker
docker-compose up
```

## VM Provisioning Flow

1. User requests VM via mobile app
2. Control plane creates DB record
3. Generates Tailscale auth key
4. Provisions Hetzner VM with cloud-init
5. VM boots, joins Tailscale network
6. Gateway starts on VM
7. VM calls back with Tailscale IP
8. User connects via WebSocket

## Security

- VMs are isolated per user
- No public SSH (Tailscale only)
- Auth keys expire after 1 hour
- WebSocket tokens are bcrypt hashed