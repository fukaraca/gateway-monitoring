.PHONY: build test clean package air install-package uninstall-package gw-logs

BINARY_NAME=gateway-monitor
BUILD_DIR=build
COVERAGE_DIR=coverage
BIN_DIR = /usr/local/bin
UNIT_DIR = /etc/systemd/system
INSTALL_USER = gwmon


build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/gateway-monitor

clean:
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)
	go clean

package: clean build test coverage
	tar -czf gateway-monitoring.tar.gz \
		--exclude='.git' \
		--exclude='*.tar.gz' \
		.

install-deps:
	go mod tidy
	go mod download

run: build
	sudo $(BUILD_DIR)/$(BINARY_NAME) --config=./config.yaml

dev:
	go run ./cmd/gateway-monitor

air: # !! generated files (build, log) will owned by root
	sudo --preserve-env=PATH "$$(command -v air)"

docker-build-test:
	docker build -f Dockerfile.test -t gw-mon-test:latest .

test: docker-build-test # both coverage and test
	docker run --rm \
		--cap-add=NET_ADMIN --cap-add=NET_RAW \
		-v "$$(pwd)":/workspace \
		-w /workspace \
		-e COVERAGE_DIR=/workspace/coverage \
		gw-mon-test:latest \
		sh -c 'mkdir -p "$$COVERAGE_DIR" \
    		&& go test -cover -covermode atomic -race -timeout 3m -coverpkg=./... -coverprofile="$$COVERAGE_DIR/coverage.out" ./... \
			&& go tool cover -html="$$COVERAGE_DIR/coverage.out" -o "$$COVERAGE_DIR/coverage.html" \
			&& go tool cover -func="$$COVERAGE_DIR/coverage.out" > "$$COVERAGE_DIR/coverage.txt"'

install-package:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/gateway-monitor
	sudo install -m 0755 $(BUILD_DIR)/$(BINARY_NAME) $(BIN_DIR)/$(BINARY_NAME)
	# create system user
	sudo id -u $(INSTALL_USER) >/dev/null 2>&1 || sudo useradd --system --no-create-home --shell /usr/sbin/nologin $(INSTALL_USER)
	sudo mkdir -p /etc/gateway-monitor /var/lib/gateway-monitor /var/log/gateway-monitor
	sudo cp ./config.yaml /etc/gateway-monitor/config.yaml
	sudo chown -R $(INSTALL_USER):$(INSTALL_USER) /etc/gateway-monitor /var/lib/gateway-monitor /var/log/gateway-monitor
	# copy service file
	sudo cp packaging/systemd/gateway-monitor.service $(UNIT_DIR)/gateway-monitor.service
	sudo setcap 'cap_net_admin,cap_net_raw+ep' $(BIN_DIR)/$(BINARY_NAME)
	sudo systemctl daemon-reload
	sudo systemctl enable --now gateway-monitor.service

uninstall-package:
	sudo systemctl stop gateway-monitor.service || true
	sudo systemctl disable gateway-monitor.service || true
	sudo rm -f $(UNIT_DIR)/gateway-monitor.service
	sudo rm -f $(BIN_DIR)/$(BINARY_NAME)
	# remove config,log etc..
	sudo rm -rf /etc/gateway-monitor /var/lib/gateway-monitor /var/log/gateway-monitor
	sudo systemctl daemon-reload

gw-logs:
	journalctl -u gateway-monitor -f

tar-it:
	tar -czf gateway-monitoring.tar.gz --exclude='./build' --exclude='./coverage' .