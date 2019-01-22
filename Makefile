TOOLS_PKG := github.com/moiot/moha

VERSION := $(shell git describe --tags --dirty)

LDFLAGS += -X "$(TOOLS_PKG)/agent.Version=$(VERSION)"
LDFLAGS += -X "$(TOOLS_PKG)/agent.BuildTS=$(shell date -u '+%Y-%m-%d %I:%M:%s')"
LDFLAGS += -X "$(TOOLS_PKG)/agent.GitHash=$(shell git rev-parse HEAD)"

GO      := GO15VENDOREXPERIMENT="1" go
GOBUILD := $(GO) build
GOTEST  := $(GO) test

GOFILTER  := grep -vE 'vendor|moctl'
GOCHECKER := $(GOFILTER) | awk '{ print } END { if (NR > 0) { exit 1 } }'

CPPLINT := cpplint --quiet --filter=-readability/casting,-build/include_subdir
CFILES := supervise/*.c

DOCKER-COMPOSE := docker-compose -p moha -f etc/docker-compose/docker-compose.yaml
PG-DOCKER-COMPOSE := docker-compose -p moha -f etc/docker-compose/postgresql/docker-compose.yaml

PACKAGES := $$(go list ./...| grep -vE 'vendor|cmd|moctl')
FILES    := $$(find . -name '*.go' -type f | grep -vE 'vendor')

TAG := $(shell git rev-parse --abbrev-ref HEAD | tr / -)-$(shell git rev-parse --short HEAD)

.PHONY: $(MAKECMDGOALS)

default: build

build: agent

clean:
	@rm -rf bin

agent:
	$(GOBUILD) -ldflags '$(LDFLAGS)' -o bin/mysql-agent cmd/mysql-agent/main.go
	$(GOBUILD) -ldflags '$(LDFLAGS)' -o bin/mysql-agent-tear-down cmd/mysql-agent-tear-down/main.go
	$(GOBUILD) -ldflags '$(LDFLAGS)' -o bin/mysql-agent-service-boot cmd/mysql-agent-service-boot/main.go
	$(GOBUILD) -ldflags '$(LDFLAGS)' -o bin/mysql-agent-service-boot-daemon cmd/mysql-agent-service-boot-daemon/main.go

docker-agent:
	mkdir -p bin/
	docker run --rm -v `pwd`:/usr/src/myapp -w /usr/src/myapp gcc:8.1.0 gcc -o bin/supervise supervise/*.c
	docker run --rm -v `pwd`:/go/src/github.com/moiot/moha -w /go/src/github.com/moiot/moha golang:1.11.0 make agent
	cp bin/* etc/docker-compose/agent/
	cp bin/* etc/docker-compose/postgresql/

checker:
	$(GOBUILD) -ldflags '$(LDFLAGS)' -o bin/mysql-agent-checker cmd/mysql-agent-checker/main.go

docker-checker:
	docker run --rm -e GOOS=`uname | tr 'A-Z' 'a-z'` -v `pwd`:/go/src/github.com/moiot/moha -w /go/src/github.com/moiot/moha golang:1.11.0 bash -c "make checker"

checker-test:
	@ docker exec mysql-node-1 mysql -h 127.0.0.1 -P 3306 -u mysql_user -pmysql_master_user_pwd -e 'select 1' >/dev/null || true
	@ until docker exec mysql-node-1 mysql -h 127.0.0.1 -P 3306 -u mysql_user -pmysql_master_user_pwd -e 'select 1' >/dev/null 2>&1; do sleep 1; echo "checker-test: Waiting for DB 3307 to come up..."; done;
	@ until docker exec mysql-node-2 mysql -h 127.0.0.1 -P 3306 -u mysql_user -pmysql_master_user_pwd -e 'select 1' >/dev/null 2>&1; do sleep 1; echo "checker-test: Waiting for DB 3308 to come up..."; done;
	@ until docker exec mysql-node-3 mysql -h 127.0.0.1 -P 3306 -u mysql_user -pmysql_master_user_pwd -e 'select 1' >/dev/null 2>&1; do sleep 1; echo "checker-test: Waiting for DB 3309 to come up..."; done;
	@ echo "grant privileges for checker user"
	docker exec mysql-node-1 mysql -S /tmp/mysql.sock  -P 3306 -u root -pmaster_root_pwd -e "grant all privileges ON *.* TO 'mysql_user'@'%'" || true
	docker exec mysql-node-2 mysql -S /tmp/mysql.sock  -P 3306 -u root -pmaster_root_pwd -e "grant all privileges ON *.* TO 'mysql_user'@'%'" || true
	docker exec mysql-node-3 mysql -S /tmp/mysql.sock  -P 3306 -u root -pmaster_root_pwd -e "grant all privileges ON *.* TO 'mysql_user'@'%'" || true
	@ echo "run with chaos " $(CHAOS)
	@ bin/mysql-agent-checker -config=checker/cfg.toml -chaos=$(CHAOS)

test:
	@echo "test"
	$(GOTEST) --race --cover -coverprofile=coverage.txt -covermode=atomic $(PACKAGES)

lint:
	@echo "gofmt check"
	@gofmt -s -l . 2>&1 | $(GOCHECKER)
	@echo "golint check"
	@golint ./agent/... 2>&1 | $(GOCHECKER)
	@golint ./pkg/... 2>&1 | $(GOCHECKER)
	@golint ./checker/... 2>&1 | $(GOCHECKER)
	@golint ./cmd/... 2>&1 | $(GOCHECKER)
	@echo "cpplint check"
	@$(CPPLINT) $(CFILES)
	@echo "done!"

update:
	dep ensure
	@echo "done!"

fmt:
	@echo "fmt"
	@go fmt ./...
	@goimports -w $(FILES)
	@echo "done!"

init:
	@ which dep >/dev/null || curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
	@ which golint >/dev/null || go get -u golang.org/x/lint/golint
	@ which goimports >/dev/null || go get -u golang.org/x/tools/cmd/goimports
	@ which cpplint >/dev/null || pip install cpplint
	@ echo "init git commit hook"
	@ if [ ! -f .git/hooks/pre-commit ]; then ln -s `pwd`/hooks/pre-commit .git/hooks; fi
	@ chmod +x .git/hooks/pre-commit
	@ echo "update testing framework"
	@ go get -u gopkg.in/check.v1

licence:
	@ python etc/copyright/copyright.py -o etc/copyright/copyright_old.txt etc/copyright/copyright.txt agent
	@ python etc/copyright/copyright.py -o etc/copyright/copyright_old.txt etc/copyright/copyright.txt pkg
	@ python etc/copyright/copyright.py -o etc/copyright/copyright_old.txt etc/copyright/copyright.txt cmd
	@ python etc/copyright/copyright.py -o etc/copyright/copyright_old.txt etc/copyright/copyright.txt checker

print-version:
	@ echo $(VERSION)

release:
	@ python release.py $(RELEASE-TAG) || echo "release should be with variable RELEASE-TAG"
	@ echo "should you not sure about what version to release, you could run 'make print-version' to find current version"

tag:
	git tag $(TAG)
	git push origin refs/tags/$(TAG)

docker-image:
	@ make docker-agent
	@ docker build -t moiot/moha:$(TAG) -f ./etc/docker-compose/agent/Dockerfile.production ./etc/docker-compose/agent


env-up:
	@ echo "start etcd cluster"
	@ $(DOCKER-COMPOSE) up --force-recreate -d etcd0 etcd1 etcd2
	@ echo "enable etcd auth"
	docker exec etcd-node-1 sh -c "echo 'root' | etcdctl --interactive=false --endpoints=http://etcd0:2379 user add root" || true
	docker exec etcd-node-1 etcdctl --user=root:root --endpoints=http://etcd0:2379 auth enable

env-down:
	@ echo "stop etcd cluster"
	@ $(DOCKER-COMPOSE) down etcd0 etcd1 etcd2 --remove-orphans

start-agents:
	$(DOCKER-COMPOSE) up --build -d mysql-test-1 mysql-test-2 mysql-test-3

stop-agents:
	$(DOCKER-COMPOSE) rm -svf mysql-test-1 mysql-test-2 mysql-test-3 || true

start-all:
	$(DOCKER-COMPOSE) up --build -d --force-recreate

pg-start-all:
	$(PG-DOCKER-COMPOSE) up --build -d --force-recreate

export CHAOS
chaos-test:
	@ make clean-data
	rm -rf logs/mysql-agent-*.log || true
	@ make env-up
	@ make start-agents
	@ make checker-test

integration-test:
	@ make docker-agent
	@ make docker-checker
	make chaos-test CHAOS=change_master
	make chaos-test CHAOS=stop_master_agent
	make chaos-test CHAOS=stop_slave_agent
	make chaos-test CHAOS=partition_one_az
	make chaos-test CHAOS=partition_master_az
	make chaos-test CHAOS=spm
	@ make clean-data

reload-agents:
	@ make docker-agent
	@ make stop-agents
	@ make start-agents

monitor:
	@ until mysql -h 127.0.0.1 -P 3307 -u mysql_user -pmysql_master_user_pwd -e 'select 1' >/dev/null 2>&1; do sleep 1; echo "Waiting for DB 3307 to come up..."; done;
	@ docker exec mysql-node-1 pmm-admin config --server pmm-server
	@ docker exec mysql-node-1 pmm-admin add mysql --user mysql_user --password mysql_master_user_pwd node-test_mysql_1
	@ until mysql -h 127.0.0.1 -P 3308 -u mysql_user -pmysql_master_user_pwd -e 'select 1' >/dev/null 2>&1; do sleep 1; echo "Waiting for DB 3308 to come up..."; done;
	@ docker exec mysql-node-2 pmm-admin config --server pmm-server
	@ docker exec mysql-node-2 pmm-admin add mysql --user mysql_user --password mysql_master_user_pwd node-test_mysql_2
	@ until mysql -h 127.0.0.1 -P 3309 -u mysql_user -pmysql_master_user_pwd -e 'select 1' >/dev/null 2>&1; do sleep 1; echo "Waiting for DB 3309 to come up..."; done;
	@ docker exec mysql-node-3 pmm-admin config --server pmm-server
	@ docker exec mysql-node-3 pmm-admin add mysql --user mysql_user --password mysql_master_user_pwd node-test_mysql_3
	@ echo "add etcd in prometheus config"
	@ docker cp ./etc/docker-compose/prometheus-etcd.yml pmm-server:/etc/
	@ docker exec pmm-server sed -i '/scrape_configs:/r /etc/prometheus-etcd.yml' /etc/prometheus.yml
	@ echo "add mysql-agent in prometheus config"
	@ docker cp ./etc/docker-compose/prometheus-mysql-agent.yml pmm-server:/etc/
	@ docker exec pmm-server sed -i '/scrape_configs:/r /etc/prometheus-mysql-agent.yml' /etc/prometheus.yml
	@ docker restart pmm-server
	@ echo "register mysql-agent"
	@ until curl 127.0.0.1:8500/v1/catalog/nodes >/dev/null 2>&1; do sleep 1; echo "Waiting for pmm-server to come up..."; done;
	@ curl -X PUT -d '{"id": "node-test_agent_1","name": "mysql-agent:metrics", "address": "mysql-node-1","port": 13306,"tags": ["mysql-agent"],"checks": [{"http": "http://mysql-node-1:13306/","interval": "5s"}]}' http://127.0.0.1:8500/v1/agent/service/register
	@ curl -X PUT -d '{"id": "node-test_agent_2","name": "mysql-agent:metrics", "address": "mysql-node-2","port": 13306,"tags": ["mysql-agent"],"checks": [{"http": "http://mysql-node-2:13306/","interval": "5s"}]}' http://127.0.0.1:8500/v1/agent/service/register
	@ curl -X PUT -d '{"id": "node-test_agent_3","name": "mysql-agent:metrics", "address": "mysql-node-3","port": 13306,"tags": ["mysql-agent"],"checks": [{"http": "http://mysql-node-3:13306/","interval": "5s"}]}' http://127.0.0.1:8500/v1/agent/service/register
	@ echo "import mysql-agent monitor dashboard"
	@ python etc/monitor/import-dashboard.py -f etc/monitor/go-processes.json
	@ python etc/monitor/import-dashboard.py -f etc/monitor/mysql-agent.json
	@ python etc/monitor/import-dashboard.py -f etc/monitor/etcd_rev3.json

demo:
	@ make clean-data
	@ make docker-agent
	@ make start-all
	@ make monitor

clean-data:
	@ $(DOCKER-COMPOSE) rm -vsf || true
	@ $(PG-DOCKER-COMPOSE) rm -vsf || true
	@ docker volume prune -f

