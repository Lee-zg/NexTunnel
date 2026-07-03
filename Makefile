.PHONY: all dev dev-server-web build package-cli package-server package-desktop package-macos lint test verify-scripts-static verify-edge verify-ebpf-linux verify-tun verify-p2p-tun verify-dashboard verify-dashboard-ssh clean help

VERSION ?= v0.6.4-alpha
WINTUN_SHA256 ?= 07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51
MAC_HOST ?= 10.160.166.44
MAC_USER ?= lizhigang
MAC_PORT ?= 22
DASHBOARD_USER ?= root
DASHBOARD_REMOTE_PORT ?= 8080

# Default target
all: build

## help: Show this help message
help:
	@echo "NexTunnel Build System"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## dev: Start Wails development server
dev:
	cd desktop && wails dev

## dev-server-web: Start server management Web console
dev-server-web:
	cd server/web && npm run dev

## build: Build the Wails desktop application
build:
	cd desktop && wails build

## package-desktop: Build Windows desktop release package
package-desktop:
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/package-desktop.ps1 -Version $(VERSION) -WintunMode bundled -WintunDllPath "$(WINTUN_DLL)" -WintunSha256 "$(WINTUN_SHA256)"

## package-macos: Build macOS desktop DMG and System TUN PKG packages on macOS
package-macos:
	bash scripts/package-macos.sh --version $(VERSION)

## package-cli: Build CLI release packages
package-cli:
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/package-cli.ps1 -Version $(VERSION)

## package-server: Build all server release packages
package-server:
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/package-server.ps1 -Version $(VERSION)

## build-server: Build all server binaries
build-server:
	cd server && go build -o ../build/control-plane ./cmd/control-plane
	cd server && go build -o ../build/relay-server ./cmd/relay
	cd server && go build -o ../build/nat-detector ./cmd/nat-detector
	cd server && go build -o ../build/dashboard ./cmd/dashboard
	cd server && go build -o ../build/edge-rehearsal ./cmd/edge-rehearsal
	cd server && go build -o ../build/ebpf-verify ./cmd/ebpf-verify
	cd cli && go build -o ../build/nextunnel .

## lint: Run all linters
lint: lint-go lint-frontend

## lint-go: Run Go linter
lint-go:
	cd desktop && golangci-lint run ./...
	cd server && golangci-lint run ./...

## lint-frontend: Run ESLint on frontend
lint-frontend:
	cd desktop/frontend && npm run lint
	cd server/web && npm run lint

## test: Run all tests
test: test-go test-frontend

## test-go: Run Go tests
test-go:
	cd desktop && go list ./... | grep -v '/frontend/node_modules/' | xargs go test
	cd installer && go test ./...
	cd server && go list ./... | grep -v '/web/node_modules/' | xargs go test
	cd cli && go test ./...
	cd pkg && go test ./...

## test-frontend: Run frontend tests
test-frontend:
	cd desktop/frontend && npm run test
	cd installer/frontend && npm run test
	cd server/web && npm run test

## verify-scripts-static: Parse PowerShell scripts, check parameter contracts, and run bash syntax checks
verify-scripts-static:
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-scripts-static.ps1

## verify-dashboard: Run Dashboard production API verification; pass DASHBOARD_URL, DASHBOARD_PASSWORD, optional DASHBOARD_ORIGIN
verify-dashboard:
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-dashboard.ps1 -BaseUrl "$(DASHBOARD_URL)" -Password "$(DASHBOARD_PASSWORD)" -AllowedOrigin "$(DASHBOARD_ORIGIN)" -ReportPath "dist/verification/dashboard-https-latest.json"

## verify-dashboard-ssh: Run Dashboard verification through SSH tunnel; pass DASHBOARD_HOST, optional DASHBOARD_USER, DASHBOARD_IDENTITY, DASHBOARD_REMOTE_PORT
verify-dashboard-ssh:
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-dashboard-ssh.ps1 -SshHost "$(DASHBOARD_HOST)" -User "$(DASHBOARD_USER)" -IdentityFile "$(DASHBOARD_IDENTITY)" -RemoteDashboardPort "$(DASHBOARD_REMOTE_PORT)" -AllowedOrigin "$(DASHBOARD_ORIGIN)" -ReportPath "dist/verification/dashboard-ssh-latest.json"

## verify-tun: Run local real TUN and route apply/reset verification
verify-tun:
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-tun.ps1

## verify-p2p-tun: Run Windows/macOS real TUN and P2P verification; pass MAC_HOST, MAC_USER, optional RELAY_ADDR, RELAY_TOKEN
verify-p2p-tun:
	pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/verify-p2p-tun.ps1 -MacHost "$(MAC_HOST)" -MacUser "$(MAC_USER)" -MacPort "$(MAC_PORT)" -RelayAddr "$(RELAY_ADDR)" -RelayToken "$(RELAY_TOKEN)"

## verify-edge: Run Edge/Anycast rehearsal; optional CONTROL_URL, CONTROL_TOKEN, REGISTER_REMOTE=true
verify-edge:
	pwsh -NoProfile -ExecutionPolicy Bypass -Command "$$argsList = @('-NoProfile','-ExecutionPolicy','Bypass','-File','scripts/verify-edge-rehearsal.ps1','-ControlUrl','$(CONTROL_URL)','-ControlToken','$(CONTROL_TOKEN)'); if ('$(REGISTER_REMOTE)' -eq 'true') { $$argsList += '-RegisterRemote' }; & pwsh @argsList"

## verify-ebpf-linux: Compile and attach XDP on Linux; pass INTERFACE_NAME=eth0 as needed
verify-ebpf-linux:
	bash scripts/verify-ebpf-linux.sh

## clean: Remove build artifacts
clean:
	rm -rf desktop/build/bin/
	rm -rf desktop/frontend/dist/
	rm -rf installer/build/bin/
	rm -rf installer/frontend/dist/
	mkdir -p installer/frontend/dist/
	touch installer/frontend/dist/.gitkeep
	rm -rf server/web/dist/
	rm -rf build/

## install-deps: Install all dependencies
install-deps:
	cd desktop && go mod tidy
	cd installer && go mod tidy
	cd server && go mod tidy
	cd pkg && go mod tidy
	cd desktop/frontend && npm install
	cd installer/frontend && npm install
	cd server/web && npm install
