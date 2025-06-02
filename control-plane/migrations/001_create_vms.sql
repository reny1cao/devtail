-- Create VMs table
CREATE TABLE IF NOT EXISTS vms (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    hetzner_id BIGINT,
    tailscale_ip VARCHAR(45),
    tailscale_auth_key TEXT,
    status VARCHAR(20) NOT NULL,
    spec JSONB NOT NULL,
    websocket_token TEXT NOT NULL,
    last_activity TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Indexes
CREATE INDEX idx_vms_user_id ON vms(user_id);
CREATE INDEX idx_vms_status ON vms(status);
CREATE INDEX idx_vms_created_at ON vms(created_at);

-- VM activity tracking table
CREATE TABLE IF NOT EXISTS vm_activity (
    id SERIAL PRIMARY KEY,
    vm_id VARCHAR(36) NOT NULL REFERENCES vms(id),
    activity_type VARCHAR(50) NOT NULL,
    details JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_vm_activity_vm_id ON vm_activity(vm_id);
CREATE INDEX idx_vm_activity_created_at ON vm_activity(created_at);