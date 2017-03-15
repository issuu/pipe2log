
BUILD_NUMBER ?= 0

BUILD_GOOS   ?= linux darwin

# need to be in the format major.minor(.revision)
APP_VERSION  := 0.9.1

REL_NAME     := pipe2log
UUID         := $(shell uuidgen)
GOPATH       ?= $(shell pwd)

SRC          := $(shell find src -type f ! -wholename "*/.git*")

GIT_HASH     := $(shell git rev-parse HEAD)
CGO_LDFLAGS  := -X main.appVersion=$(APP_VERSION)-$(BUILD_NUMBER) -X main.appGitHash=$(GIT_HASH) -X main.appBuildTime=$(shell date -u +%Y-%m-%d:%H.%M.%S)

# create static linked binary for linux based systems
linux_LDFLAGS  = "-linkmode external -extldflags '-static' $(CGO_LDFLAGS)"
darwin_LDFLAGS = "$(CGO_LDFLAGS)"

#
# for local development
#
bin/$(REL_NAME): $(SRC)
	cd src/github.com/issuu/pipe2log ; \
	GOPATH=$(GOPATH) go get -v -d ; \
	GOPATH=$(GOPATH) go install -v -ldflags "$(CGO_LDFLAGS)"

clean:
	-rm -fr _rel/* equivs/pipe2log.control
	@echo make target $@ done

release: clean
release: $(foreach GOOS,$(BUILD_GOOS),_rel/$(REL_NAME)_$(GOOS))
release:
	@echo make target $@ done

_rel/$(REL_NAME)_%: Makefile src/github.com/issuu/pipe2log/Dockerfile.build $(SRC)
_rel/$(REL_NAME)_%: GOOS=$*
_rel/$(REL_NAME)_%: GOOS_LDFLAGS=$($(GOOS)_LDFLAGS)
_rel/$(REL_NAME)_%:
	-mkdir -p _rel
	docker build \
		-t $(REL_NAME)-$(GOOS):$(UUID) \
		--build-arg CGO_LDFLAGS=$(GOOS_LDFLAGS) --build-arg GOOS=$(GOOS) \
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
	github-release.sh -t v$(APP_VERSION)-$(BUILD_NUMBER) -o issuu -r pipe2log -a $(WORKSPACE)/_rel 
	@echo make target $@ done

equivs/pipe2log.control: equivs/pipe2log.template
	sed 's/{{Version}}/$(APP_VERSION)-$(BUILD_NUMBER)/' $< > $@
	@echo make target $@ done

debian: _rel/pipe2log_linux equivs/pipe2log.control
	cp $< equivs/pipe2log
	cd equivs ; equivs-build pipe2log.control ; rm pipe2log ; mv pipe2log_$(APP_VERSION)-$(BUILD_NUMBER)_*.deb ../_rel/
	which commit-deb2repo.sh && commit-deb2repo.sh -r infrastructure _rel/pipe2log_$(APP_VERSION)-$(BUILD_NUMBER)_*.deb
	-rm equivs/pipe2log.control
	@echo make target $@ done

