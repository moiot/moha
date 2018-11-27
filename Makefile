TOOLS_PKG := git.mobike.io/database/mysql-agent

VERSION := $(shell git describe --tags --dirty)

LDFLAGS += -X "$(TOOLS_PKG)/agent.Version=$(VERSION)"
LDFLAGS += -X "$(TOOLS_PKG)/agent.BuildTS=$(shell date -u '+%Y-%m-%d %I:%M:%s')"
LDFLAGS += -X "$(TOOLS_PKG)/agent.GitHash=$(shell git rev-parse HEAD)"

GO      := GO15VENDOREXPERIMENT="1" go
GOBUILD := $(GO) build
GOTEST  := $(GO) test

GOFILTER  := grep -vE 'vendor'
GOCHECKER := $(GOFILTER) | awk '{ print } END { if (NR > 0) { exit 1 } }'

CPPLINT := cpplint --quiet --filter=-readability/casting,-build/include_subdir
CFILES := supervise/*.c

PACKAGES := $$(go list ./...| grep -vE 'vendor|cmd')
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

docker-agent:
	mkdir -p bin/
	docker run --rm -v `pwd`:/usr/src/myapp -w /usr/src/myapp docker.mobike.io/databases/gcc:8.1.0 gcc -o bin/supervise supervise/*.c
	docker run --rm -v `pwd`:/go/src/git.mobike.io/database/mysql-agent -w /go/src/git.mobike.io/database/mysql-agent docker.mobike.io/databases/golang:1.9.2 make agent
	cp bin/* etc/docker-compose/agent/

checker:
	$(GOBUILD) -ldflags '$(LDFLAGS)' -o bin/mysql-agent-checker cmd/mysql-agent-checker/main.go

checker-test:
	@ bin/mysql-agent-checker -config=checker/cfg.toml

docker-checker-test:
	docker run --rm -e GOOS=`uname | tr 'A-Z' 'a-z'` -v `pwd`:/go/src/git.mobike.io/database/mysql-agent -w /go/src/git.mobike.io/database/mysql-agent docker.mobike.io/databases/golang:1.9.2 bash -c "make checker"
	make checker-test

test:
	@echo "test"
	$(GOTEST) --race --cover $(PACKAGES)

lint:
	@echo "golint check"
	@gofmt -s -l . 2>&1 | $(GOCHECKER)
	@golint ./... 2>&1 | $(GOCHECKER)
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
	@ echo "please run 'python release.py <version>' to release"
	@ echo "should you not sure about what version to release, you could run 'make print-version' to find current version"

tag:
	git tag $(TAG)
	git push origin refs/tags/$(TAG)
	git push github refs/tags/$(TAG) || true

docker-image:
	@ make docker-agent
	@ docker build -t docker.mobike.io/databases/mysql-agent:$(TAG) ./etc/docker-compose/agent

env-up:
	@ echo "start etcd cluster"
	@ docker-compose -p mysqlagent -f etc/docker-compose/docker-compose.yaml up --force-recreate -d etcd0 etcd1 etcd2
	@ echo "enable etcd auth"
	docker exec etcd-node-1 sh -c "echo 'root' | etcdctl --interactive=false --endpoints=http://etcd0:2379 user add root" || true
	docker exec etcd-node-1 etcdctl --user=root:root --endpoints=http://etcd0:2379 auth enable

env-down:
	@ echo "stop etcd cluster"
	@ docker-compose -p mysqlagent -f etc/docker-compose/docker-compose.yaml down etcd0 etcd1 etcd2 --remove-orphans

start-agents:
	docker-compose -p mysqlagent -f etc/docker-compose/docker-compose.yaml up --build -d mysql-test-1 mysql-test-2 mysql-test-3

stop-agents:
	docker-compose -p mysqlagent -f etc/docker-compose/docker-compose.yaml rm -svf mysql-test-1 mysql-test-2 mysql-test-3 || true

start-all:
	docker-compose -p mysqlagent -f etc/docker-compose/docker-compose.yaml up --build -d --force-recreate

integration-test:
	@ make clean-data
	@ make env-up
	@ make reload-agents
	@ make docker-checker-test
	@ make clean-data

reload-agents:
	@ make docker-agent
	@ make stop-agents
	@ make start-agents

monitor:
	@ until mysql -h 127.0.0.1 -P 3307 -u root -pmaster_root_pwd -e 'select 1' >/dev/null 2>&1; do sleep 1; echo "Waiting for DB 3307 to come up..."; done;
	@ docker exec mysql-node-2 pmm-admin config --server pmm-server
	@ docker exec mysql-node-2 pmm-admin add mysql --user root --password master_root_pwd node-test_mysql_2
	@ until mysql -h 127.0.0.1 -P 3308 -u root -pmaster_root_pwd -e 'select 1' >/dev/null 2>&1; do sleep 1; echo "Waiting for DB 3308 to come up..."; done;
	@ docker exec mysql-node-1 pmm-admin config --server pmm-server
	@ docker exec mysql-node-1 pmm-admin add mysql --user root --password master_root_pwd node-test_mysql_1
	@ until mysql -h 127.0.0.1 -P 3309 -u root -pmaster_root_pwd -e 'select 1' >/dev/null 2>&1; do sleep 1; echo "Waiting for DB 3309 to come up..."; done;
	@ docker exec mysql-node-3 pmm-admin config --server pmm-server
	@ docker exec mysql-node-3 pmm-admin add mysql --user root --password master_root_pwd node-test_mysql_3
	@ echo "add mysql-agent in prometheus config"
	@ docker exec -w /etc pmm-server sed -i '/scrape_configs:/r prometheus-mysql-agent.yml' prometheus.yml
	@ docker restart pmm-server
	@ echo "register mysql-agent"
	@ until curl 127.0.0.1:8500/v1/catalog/nodes >/dev/null 2>&1; do sleep 1; echo "Waiting for pmm-server to come up..."; done;
	@ curl -X PUT -d '{"id": "node-test_agent_1","name": "mysql-agent:metrics", "address": "mysql-node-1","port": 13306,"tags": ["mysql-agent"],"checks": [{"http": "http://mysql-node-1:13306/","interval": "5s"}]}' http://127.0.0.1:8500/v1/agent/service/register
	@ curl -X PUT -d '{"id": "node-test_agent_2","name": "mysql-agent:metrics", "address": "mysql-node-2","port": 13306,"tags": ["mysql-agent"],"checks": [{"http": "http://mysql-node-2:13306/","interval": "5s"}]}' http://127.0.0.1:8500/v1/agent/service/register
	@ curl -X PUT -d '{"id": "node-test_agent_3","name": "mysql-agent:metrics", "address": "mysql-node-3","port": 13306,"tags": ["mysql-agent"],"checks": [{"http": "http://mysql-node-3:13306/","interval": "5s"}]}' http://127.0.0.1:8500/v1/agent/service/register
	@ echo "import mysql-agent monitor dashboard"
	@ python etc/monitor/import-dashboard.py -f  etc/monitor/go-processes.json
	@ python etc/monitor/import-dashboard.py -f  etc/monitor/mysql-agent.json

demo:
	@ make clean-data
	@ make docker-agent
	@ make start-all
	@ make monitor

clean-data:
	@ docker stop mysql-node-1 || true
	@ docker rm mysql-node-1 || true
	@ docker volume rm mysqlagent_mysql-node-1-data || true
	@ docker stop mysql-node-2 || true
	@ docker rm mysql-node-2 || true
	@ docker volume rm mysqlagent_mysql-node-2-data || true
	@ docker stop mysql-node-3 || true
	@ docker rm mysql-node-3 || true
	@ docker volume rm mysqlagent_mysql-node-3-data || true
	@ docker stop etcd-node-0 || true
	@ docker stop etcd-node-1 || true
	@ docker stop etcd-node-2 || true
	@ docker rm etcd-node-0 || true
	@ docker rm etcd-node-1 || true
	@ docker rm etcd-node-2 || true
	@ docker volume rm mysqlagent_etcd0 || true
	@ docker volume rm mysqlagent_etcd1 || true
	@ docker volume rm mysqlagent_etcd2 || true
	@ docker stop pmm-server || true
	@ docker rm pmm-server || true
	@ docker volume rm mysqlagent_pmm-data-prometheus || true
	@ docker volume rm mysqlagent_pmm-data-mysql || true
	@ docker volume rm mysqlagent_pmm-data-grafana || true
	@ docker volume rm mysqlagent_pmm-data-consul || true

