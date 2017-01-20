
BUILD_NUMBER ?= 0

BUILD_GOOS   := linux darwin

APP_VERSION  := 0.9

REL_NAME     := pipe2log
UUID         := $(shell uuidgen)
GOPATH       ?= $(shell pwd)

SRC          := $(shell find src -type f ! -wholename "*/.git*")

GIT_HASH     := $(shell git rev-parse HEAD)
CGO_LDFLAGS  := "-X main.appVersion=$(APP_VERSION).$(BUILD_NUMBER) -X main.appGitHash=$(GIT_HASH) -X main.appBuildTime=$(shell date -u +%Y-%m-%d:%H.%M.%S)"

#
# for local development
#
bin/$(REL_NAME): $(SRC)
	cd src/github.com/issuu/pipe2log ; \
	GOPATH=$(GOPATH) go get -v -d ; \
	GOPATH=$(GOPATH) go install -ldflags $(CGO_LDFLAGS) -v

clean:
	-rm -fr _rel/*
	@echo make target $@ done

release: clean
release: $(foreach GOOS,$(BUILD_GOOS),_rel/$(REL_NAME)_$(GOOS))
release:
	@echo make target $@ done

_rel/$(REL_NAME)_%: Makefile src/github.com/issuu/pipe2log/Dockerfile.build $(SRC)
_rel/$(REL_NAME)_%: GOOS=$*
_rel/$(REL_NAME)_%:
	-mkdir -p _rel
	docker build \
		-t $(REL_NAME)-$(GOOS):$(UUID) \
		--build-arg CGO_LDFLAGS=$(CGO_LDFLAGS) --build-arg GOOS=$(GOOS) \
		-f src/github.com/issuu/pipe2log/Dockerfile.build \
		src/github.com/issuu/pipe2log
	docker run --name $(REL_NAME)-$(GOOS)-$(UUID) $(REL_NAME)-$(GOOS):$(UUID) echo running
	docker cp $(REL_NAME)-$(GOOS)-$(UUID):/go/bin/$(GOOS)_amd64/app $@ || \
		docker cp $(REL_NAME)-$(GOOS)-$(UUID):/go/bin/app $@
	docker rm -f $(REL_NAME)-$(GOOS)-$(UUID)
	@echo compiled new version
	test $(shell uname -s | tr '[:upper:]' '[:lower:]') = $(GOOS) && $@ --version || :
	file $@
	@echo make target $@ done

github-release:
	@# hoping the jenkins github plugin will be able to do this in the future.
	github-release.sh -t v$(APP_VERSION).$(BUILD_NUMBER) -o issuu -r pipe2log -a $(WORKSPACE)/_rel 
	@echo make target $@ done

