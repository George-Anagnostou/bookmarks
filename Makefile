GO ?= go
DIST_DIR ?= dist
SERVER_GOOS ?= linux
SERVER_GOARCH ?= amd64
SERVER_BIN := $(DIST_DIR)/bookmarkd-$(SERVER_GOOS)-$(SERVER_GOARCH)

-include .env.deploy

export BOOKMARKS_DEPLOY_HOST
export BOOKMARKS_DEPLOY_SERVICE
export BOOKMARKS_DEPLOY_TARGET
export BOOKMARKS_DEPLOY_USER
export BOOKMARKS_DOMAIN
export BOOKMARKS_URL

.PHONY: test build-server build-cli update rollback clean

test:
	$(GO) test ./...

build-server:
	mkdir -p $(DIST_DIR)
	GOOS=$(SERVER_GOOS) GOARCH=$(SERVER_GOARCH) $(GO) build -trimpath -o $(SERVER_BIN) ./cmd/bookmarkd

update: test build-cli build-server
	./scripts/deploy-bookmarkd.sh $(SERVER_BIN)

rollback:
	./scripts/rollback-bookmarkd.sh

build-cli:
	mkdir -p $(DIST_DIR)
	$(GO) install -trimpath ./cmd/bookmarkctl

clean:
	rm -rf $(DIST_DIR)
