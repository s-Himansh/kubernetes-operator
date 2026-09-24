IMG ?= ghcr.io/s-himansh/kubernetes-operator:latest
KUSTOMIZE ?= kustomize
CONTROLLER_GEN ?= controller-gen
ENVTEST ?= setup-envtest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

.PHONY: all
all: build

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: manifests
manifests: controller-gen ## Generate CRD + RBAC
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate DeepCopy
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run envtest
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use --bin-dir $(shell pwd)/testbin -p path)" go test -v -race -coverprofile=coverage.txt ./...

.PHONY: build
build: generate fmt vet ## Build manager binary
	go build -o bin/manager ./cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run from host
	go run ./cmd/main.go --enable-webhook=false

.PHONY: docker-build
docker-build: ## Build docker image
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image
	docker push ${IMG}

.PHONY: install
install: manifests kustomize ## Install CRDs
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found -f -

.PHONY: helm-template
helm-template: ## Render Helm
	helm template sharded-cache ./charts/kubernetes-operator --set image.repository=ghcr.io/s-himansh/kubernetes-operator --set image.tag=latest

CONTROLLER_GEN_BIN := $(GOBIN)/controller-gen
.PHONY: controller-gen
controller-gen:
	@test -s $(CONTROLLER_GEN_BIN) || go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.15.0

KUSTOMIZE_BIN := $(GOBIN)/kustomize
.PHONY: kustomize
kustomize:
	@test -s $(KUSTOMIZE_BIN) || go install sigs.k8s.io/kustomize/kustomize/v5@v5.4.2

ENVTEST_BIN := $(GOBIN)/setup-envtest
.PHONY: envtest
envtest:
	@test -s $(ENVTEST_BIN) || go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: lint
lint:
	golangci-lint run ./... || echo "golangci-lint not installed, skipping"
