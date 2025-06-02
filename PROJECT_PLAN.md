# DevTail: Mobile-First AI Cloud Development Platform

## Vision
Build a mobile-first, AI-powered cloud development platform that combines Codespaces-grade cloud dev, Copilot-level AI assistance, and Termius-style mobile UX into one frictionless app. Code with an AI pair on 5G, seamlessly handoff to desktop, never lose context.

## MVP Target (3 months)
### Phase 1: Foundation (Month 1)
- [ ] **Native Mobile Terminal**
  - Native iOS/Android terminal renderer (not WebView)
  - Sub-50ms keystroke latency
  - Hardware keyboard support
  - Gesture controls (pinch zoom, swipe for tabs)
  
- [ ] **Tailscale Integration**
  - Direct P2P connections using Tailscale SDK
  - Connection persistence across network changes
  - Quick reconnect with state preservation
  
- [ ] **Basic AI Chat**
  - Simple Aider integration in side panel
  - Claude API for code assistance
  - Markdown rendering for responses

### Phase 2: Core Features (Month 2)
- [ ] **Performance Optimization**
  - WebSocket protocol compression
  - Predictive input buffering
  - Background connection maintenance
  - Battery usage optimization
  
- [ ] **File Management**
  - Built-in file browser
  - Syntax-highlighted code viewer
  - Quick edit with AI suggestions
  - Git status integration
  
- [ ] **Session Management**
  - Multiple concurrent sessions
  - Tab-based UI like Termius
  - Session state persistence
  - Quick switch gestures

### Phase 3: Polish & Launch (Month 3)
- [ ] **Mobile UX Polish**
  - Custom keyboard with programming keys
  - Snippet management
  - Theme customization
  - Font selection
  
- [ ] **Security & Auth**
  - Biometric authentication
  - Encrypted credential storage
  - Tailscale key management
  - Session recording (optional)
  
- [ ] **Monetization**
  - Free tier: 1 VM, basic features
  - Pro tier: Unlimited VMs, AI usage included
  - Team tier: Shared VMs, collaboration

## Post-MVP Roadmap

### Q2 2025: Advanced Features
- [ ] Voice-to-code commands
- [ ] Collaborative sessions
- [ ] IDE integration (VS Code Server)
- [ ] Custom AI model fine-tuning

### Q3 2025: Enterprise
- [ ] SSO integration
- [ ] Audit logging
- [ ] Compliance features
- [ ] On-premise deployment option

## Technical Architecture

### 30-Second Architecture
```
Mobile (React Native + Floating AI Bar)
    ↓ WSS 443
Gateway (Go) ← REST/SSE → AI (Aider + LLM API)
    ↓ pty
tmux session ← bash/vim → DevBox (Docker/Firecracker Ubuntu 22.04)
    ↓ HTTP  
openvscode-server (Desktop IDE)
```

### Current Implementation Status

#### ✅ Implemented Components
- **Gateway**: Go WebSocket multiplexer with terminal/chat handling
- **Control Plane**: Hetzner VM provisioning with Tailscale networking  
- **Terminal**: PTY management with session support
- **Chat**: Mock Aider integration (placeholder)
- **Protocol**: JSON message types, compression ready

#### 🔴 Critical Gaps
- **Real Aider Integration**: Replace mock-aider.py with actual CLI
- **Mobile App**: React Native with floating assistant UI
- **Protocol Buffers**: Binary protocol for mobile efficiency
- **tmux Integration**: Session persistence across disconnects
- **File Sync**: Diff-based file operations protocol
- **IDE Server**: openvscode-server integration
- **Resilience**: Connection buffering and reconnect logic

#### 🟡 Enhancement Needed
- **AI Cost Control**: Token limits, model tier routing
- **Global Connectivity**: China proxy, CDN endpoints
- **Performance**: Sub-50ms latency optimizations
- **Security**: JWT authentication, encrypted storage

## Success Metrics
- **Performance**: <50ms input latency, <2s connection time
- **Reliability**: 99.9% uptime, <1% dropped connections
- **Battery**: <5% battery/hour active use
- **User Growth**: 1K MAU in 3 months, 10K in 6 months
- **Revenue**: $10K MRR within 6 months

## Risk Mitigation
1. **Terminal Performance**: Use native libraries (libvterm), not web tech
2. **AI Costs**: Implement smart caching and prompt optimization
3. **Mobile Platform Restrictions**: Follow App Store guidelines strictly
4. **Competition**: Focus on integration quality, not feature parity

## Implementation Roadmap

### Month 1: Core Backend Foundation
**Week 1-2: Real AI Integration**
- Replace mock-aider.py with actual Aider CLI integration
- Implement subprocess management with PTY handling
- Add context management for long AI conversations
- Build file watching for Aider's live edits

**Week 3-4: Protocol & Performance**  
- Implement Protocol Buffer binary messaging
- Add WebSocket compression (zstd for mobile)
- Build connection state machine with exponential backoff
- Create session resume protocol with message buffering

### Month 2: Mobile & File Operations
**Week 5-6: React Native Foundation**
- Native terminal renderer (not WebView)
- Floating AI assistant UI implementation  
- Tailscale SDK integration for direct P2P
- Hardware keyboard support and gesture controls

**Week 7-8: File Sync & tmux**
- Efficient diff-based file sync protocol
- tmux session management and persistence
- Git integration for status/diff monitoring
- Syntax-aware diff generation

### Month 3: Production Ready
**Week 9-10: IDE & Cross-device**
- openvscode-server integration for desktop
- JWT authentication and session management
- Cross-device handoff with state preservation
- Performance optimization (<50ms latency)

**Week 11-12: Polish & Launch**
- AI cost controls (token limits, model routing)
- Global connectivity (CDN, China proxy setup)
- Mobile app store preparation
- Monitoring and analytics dashboard

## Immediate Next Steps (This Week)
1. **Fix Aider Integration** - Priority #1 to enable real AI functionality
2. **Protocol Buffer Implementation** - Mobile efficiency foundation
3. **tmux Session Management** - Connection resilience core
4. **File Sync Protocol Design** - Developer workflow enabler

## Resource Requirements
- 2 senior mobile developers (iOS/Android)
- 1 backend engineer (Go/WebSocket optimization)
- 1 DevOps engineer (infrastructure/monitoring)
- UI/UX designer familiar with developer tools
- $5K/month for infrastructure and AI API costs