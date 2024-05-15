# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= $(shell cat VERSION)
VERSION_TAG ?= v$(VERSION)
# Replaces Operator version
# Set this when when there is a new patch release in the channel.
REPLACES_VERSION ?= $(shell cat REPLACES_VERSION)

GATEKEEPER_VERSION ?= $(shell cat GATEKEEPER_VERSION || cat VERSION)

LOCAL_BIN ?= $(PWD)/ci-tools/bin
export PATH := $(LOCAL_BIN):$(PATH)

# Detect the OS to set per-OS defaults
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# Fix sed issues on mac by using GSED and fix base64 issues on macos by omitting the -w 0 parameter
SED = sed
ifeq ($(GOOS), darwin)
  SED = gsed
endif

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
CHANNELS ?= stable,$(shell echo $(VERSION) | cut -d '.' -f 1-2)
ifneq ($(origin CHANNELS), undefined)
  BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
DEFAULT_CHANNEL ?= stable
ifneq ($(origin DEFAULT_CHANNEL), undefined)
  BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)
# Option to use podman or docker
DOCKER ?= docker

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# gatekeeper.sh/gatekeeper-operator-bundle:$VERSION and gatekeeper.sh/gatekeeper-operator-catalog:$VERSION.
REPO ?= quay.io/gatekeeper
IMAGE_TAG_BASE ?= $(REPO)/gatekeeper-operator

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:$(VERSION_TAG)

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):$(VERSION_TAG)
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = $(KUBERNETES_VERSION:v%=%)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
  GOBIN=$(shell go env GOPATH)/bin
else
  GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: clean
clean: IMG = quay.io/gatekeeper/gatekeeper-operator:$(VERSION_TAG)
clean: delete-test-cluster clean-manifests ## Clean up build artifacts.
	rm $(LOCAL_BIN)/*

.PHONY: clean-manifests
clean-manifests: IMG = quay.io/gatekeeper/gatekeeper-operator:$(VERSION_TAG)
clean-manifests: kustomize unpatch-deployment bundle
	# Reset all kustomization.yaml files
	git restore **/kustomization.yaml

##@ Development

CONTROLLER_GEN = $(LOCAL_BIN)/controller-gen
KUSTOMIZE = $(LOCAL_BIN)/kustomize
ENVTEST = $(LOCAL_BIN)/setup-envtest
GO_BINDATA = $(LOCAL_BIN)/go-bindata
GINKGO = $(LOCAL_BIN)/ginkgo
KUSTOMIZE_VERSION ?= v5.0.1
OPM_VERSION ?= v1.27.0
GO_BINDATA_VERSION ?= v3.1.2+incompatible
OLM_VERSION ?= v0.25.0
KUBERNETES_VERSION ?= v1.28.0

.PHONY: e2e-dependencies
e2e-dependencies:
	$(call go-get-tool,github.com/onsi/ginkgo/v2/ginkgo@$(shell awk '/github.com\/onsi\/ginkgo\/v2/ {print $$2}' go.mod))

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases output:rbac:dir=config/rbac

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	GOFLAGS=$(GOFLAGS) go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	GOFLAGS=$(GOFLAGS) go vet ./...

.PHONY: tidy
tidy: ## Run go mod tidy
	GO111MODULE=on GOFLAGS=$(GOFLAGS) go mod tidy

.PHONY: test
test: manifests generate fmt vet envtest test-unit ## Run tests.

.PHONY: test-unit
test-unit:
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" GOFLAGS=$(GOFLAGS) go test $(TESTARGS) $$(go list ./... | grep -v /test/)

.PHONY: test-coverage
test-coverage: TESTARGS = -json -cover -covermode=atomic -coverprofile=coverage_unit.out
test-coverage: test-unit

E2E_LABEL_FILTER = --label-filter="!openshift"
.PHONY: test-e2e
test-e2e: e2e-dependencies generate fmt vet ## Run e2e tests, using the configured Kubernetes cluster in ~/.kube/config
	GOFLAGS=$(GOFLAGS) USE_EXISTING_CLUSTER=true $(GINKGO) -v --trace --fail-fast $(E2E_LABEL_FILTER) ./test/e2e -- --namespace="$(NAMESPACE)" --timeout="5m" --delete-timeout="10m"

.PHONY: test-e2e-openshift
test-e2e-openshift: E2E_LABEL_FILTER = --label-filter="openshift"
test-e2e-openshift: NAMESPACE = openshift-gatekeeper-system
test-e2e-openshift: test-e2e 

.PHONY: test-openshift-setup
test-openshift-setup: 
	kubectl apply -f https://raw.githubusercontent.com/openshift/api/release-4.15/route/v1/route.crd.yaml
	kubectl create ns openshift-gatekeeper-system

.PHONY: test-cluster
test-cluster: ## Create a local kind cluster with a registry for testing
	KIND_CLUSTER_VERSION=$(KUBERNETES_VERSION) ./scripts/kind-with-registry.sh

KIND_CLUSTER_NAME ?= kind
.PHONY: delete-test-cluster
delete-test-cluster: ## Clean up the local kind cluster and registry
	# Stopping and removing the registry container
	-docker stop $(shell docker inspect -f '{{.Id}}' kind-registry 2>/dev/null || printf "-")
	-docker rm $(shell docker inspect -f '{{.Id}}' kind-registry 2>/dev/null || printf "-")
	kind delete cluster --name "$(KIND_CLUSTER_NAME)"

.PHONY: test-gatekeeper-e2e
test-gatekeeper-e2e: ## Applies the test yaml and verifies that BATS is installed. For use by GitHub Actions
	kubectl -n $(NAMESPACE) apply -f ./config/samples/gatekeeper_e2e_test.yaml
	bats --version

BATS := $(LOCAL_BIN)/bats
BATS_VERSION ?= 1.8.2
.PHONY: download-binaries
download-binaries: kustomize go-bindata envtest controller-gen
	# Checking installation of bats v$(BATS_VERSION)
	@if [[ ! -f $(BATS) ]] || [[ "$(shell $(BATS) --version)" != "Bats $(BATS_VERSION)" ]]; then \
		echo "Downloading and installing bats"; \
		curl -sSLO https://github.com/bats-core/bats-core/archive/v${BATS_VERSION}.tar.gz; \
		tar -zxf v${BATS_VERSION}.tar.gz; \
		bash bats-core-${BATS_VERSION}/install.sh $(PWD)/ci-tools; \
		rm -rf bats-core-${BATS_VERSION} v${BATS_VERSION}.tar.gz; \
	fi

.PHONY: kind-bootstrap-cluster
kind-bootstrap-cluster: test-cluster install dev-build
	kubectl label ns $(NAMESPACE)  --overwrite pod-security.kubernetes.io/audit=privileged
	kubectl label ns $(NAMESPACE)  --overwrite pod-security.kubernetes.io/enforce=privileged
	kubectl label ns $(NAMESPACE)  --overwrite pod-security.kubernetes.io/warn=privileged
	kind load docker-image $(IMG)
	$(MAKE) deploy-ci NAMESPACE=$(NAMESPACE) IMG=$(IMG)
	kubectl -n $(NAMESPACE) wait deployment/gatekeeper-operator-controller --for condition=Available --timeout=90s

.PHONY: build
build: generate fmt vet ## Build manager binary.
	CGO_ENABLED=1 GOFLAGS=$(GOFLAGS) go build -ldflags $(LDFLAGS) -o bin/manager main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host, using the configured Kubernetes cluster in ~/.kube/config
	GOFLAGS=$(GOFLAGS) GATEKEEPER_TARGET_NAMESPACE=$(NAMESPACE) go run -ldflags $(LDFLAGS) ./main.go

.PHONY: docker-build
docker-build: GOOS = linux
docker-build: GOARCH = amd64
docker-build: test ## Build docker image with the manager.
	# Building with --platform=linux/amd64 because the image-builder is only built for that architecture: https://github.com/stolostron/image-builder
	$(DOCKER) build --platform $(GOOS)/$(GOARCH) --build-arg GOOS=$(GOOS) --build-arg GOARCH=$(GOARCH) --build-arg LDFLAGS=${LDFLAGS} -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(DOCKER) push ${IMG}

BINDATA_OUTPUT_FILE := ./pkg/bindata/bindata.go

.PHONY: go-bindata
go-bindata:
	$(call go-get-tool,github.com/go-bindata/go-bindata/go-bindata@${GO_BINDATA_VERSION})

.PHONY: .run-bindata
.run-bindata: go-bindata
	mkdir -p ./$(GATEKEEPER_MANIFEST_DIR)-rendered && \
	$(KUSTOMIZE) build $(GATEKEEPER_MANIFEST_DIR) -o ./$(GATEKEEPER_MANIFEST_DIR)-rendered && \
	$(GO_BINDATA) -nocompress -nometadata \
		-prefix "bindata" \
		-pkg "bindata" \
		-o "$${BINDATA_OUTPUT_PREFIX}$(BINDATA_OUTPUT_FILE)" \
		-ignore "OWNERS" \
		./$(GATEKEEPER_MANIFEST_DIR)-rendered/... && \
	rm -rf ./$(GATEKEEPER_MANIFEST_DIR)-rendered && \
	gofmt -s -w "$${BINDATA_OUTPUT_PREFIX}$(BINDATA_OUTPUT_FILE)"

.PHONY: update-bindata
update-bindata:
	$(MAKE) .run-bindata ;\

.PHONY: verify-bindata
verify-bindata:
	export TMP_DIR=$$(mktemp -d) ;\
	export BINDATA_OUTPUT_PREFIX="$${TMP_DIR}/" ;\
	$(MAKE) .run-bindata ;\
	if ! diff -Naup {.,$${TMP_DIR}}/$(BINDATA_OUTPUT_FILE); then \
		echo "Error: $(BINDATA_OUTPUT_FILE) and $${TMP_DIR}/$(BINDATA_OUTPUT_FILE) files differ. Run 'make update-bindata' and try again." ;\
		rm -rf "$${TMP_DIR}" ;\
		exit 1 ;\
	fi ;\
	rm -rf "$${TMP_DIR}" ;\

.PHONY: release
release: manifests kustomize patch-deployment
	$(KUSTOMIZE) build config/default > ./deploy/gatekeeper-operator.yaml

##@ Deployment

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: manifests kustomize patch-deployment apply-manifests unpatch-deployment ## Deploy controller to the K8s cluster specified in ~/.kube/config.

.PHONY: apply-manifests
apply-manifests:
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: patch-deployment
patch-deployment:
	cd config/default && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}

.PHONY: unpatch-deployment
unpatch-deployment: 
	cd config/default && $(KUSTOMIZE) edit set namespace gatekeeper-system
	cd config/manager && $(KUSTOMIZE) edit set image controller=quay.io/gatekeeper/gatekeeper-operator:$(VERSION_TAG)

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

.PHONY: deploy-ci
deploy-ci: install patch-image deploy unpatch-image ## Deploys the controller with a patched pull policy.

.PHONY: deploy-olm
deploy-olm:
	$(OPERATOR_SDK) olm install --version $(OLM_VERSION) --timeout 5m

.PHONY: deploy-using-olm
deploy-using-olm:
	$(SED) -i 's#quay.io/gatekeeper/gatekeeper-operator-bundle-index:latest#$(BUNDLE_INDEX_IMG)#g' config/olm-install/kustomization.yaml
	$(SED) -i 's#channel: stable#channel: $(DEFAULT_CHANNEL)#g' config/olm-install/kustomization.yaml
	cd config/olm-install && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
	$(KUSTOMIZE) build config/olm-install | kubectl apply -f -

.PHONY: patch-image
patch-image: ## Patches the manager's image pull policy to be IfNotPresent.
	$(SED) -i 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' config/manager/manager.yaml

.PHONY: unpatch-image
unpatch-image: ## Patches the manager's image pull policy to be Always.
	$(SED) -i 's/imagePullPolicy: IfNotPresent/imagePullPolicy: Always/g' config/manager/manager.yaml

.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,sigs.k8s.io/controller-tools/cmd/controller-gen@v0.6.1)

.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,sigs.k8s.io/kustomize/kustomize/v5@${KUSTOMIZE_VERSION})

.PHONY: envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

# go-get-tool will 'go install' any package $1 and install it to LOCAL_BIN.
define go-get-tool
  @set -e ;\
  echo "Checking installation of $(1)" ;\
  GOBIN=$(LOCAL_BIN) go install $(1)
endef

##@ Operator Bundling

.PHONY: bundle
bundle: operator-sdk manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	# Set base64data in CSV with SVG logo: $(SED) -i 's/base64data: ""/base64data: "<base64-string>"/g' bundle/manifests/gatekeeper-operator.clusterserviceversion.yaml 
	@$(SED) -i 's/base64data: \"\"/base64data: \"PHN2ZyBpZD0iZjc0ZTM5ZDEtODA2Yy00M2E0LTgyZGQtZjM3ZjM1NWQ4YWYzIiBkYXRhLW5hbWU9Ikljb24iIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyIgdmlld0JveD0iMCAwIDM2IDM2Ij4KICA8ZGVmcz4KICAgIDxzdHlsZT4KICAgICAgLmE0MWM1MjM0LWExNGEtNGYzZC05MTYwLTQ0NzJiNzZkMDA0MCB7CiAgICAgICAgZmlsbDogI2UwMDsKICAgICAgfQogICAgPC9zdHlsZT4KICA8L2RlZnM+CiAgPGc+CiAgICA8cGF0aCBjbGFzcz0iYTQxYzUyMzQtYTE0YS00ZjNkLTkxNjAtNDQ3MmI3NmQwMDQwIiBkPSJNMjUsMTcuMzhIMjMuMjNhNS4yNyw1LjI3LDAsMCwwLTEuMDktMi42NGwxLjI1LTEuMjVhLjYyLjYyLDAsMSwwLS44OC0uODhsLTEuMjUsMS4yNWE1LjI3LDUuMjcsMCwwLDAtMi42NC0xLjA5VjExYS42Mi42MiwwLDEsMC0xLjI0LDB2MS43N2E1LjI3LDUuMjcsMCwwLDAtMi42NCwxLjA5bC0xLjI1LTEuMjVhLjYyLjYyLDAsMCwwLS44OC44OGwxLjI1LDEuMjVhNS4yNyw1LjI3LDAsMCwwLTEuMDksMi42NEgxMWEuNjIuNjIsMCwwLDAsMCwxLjI0aDEuNzdhNS4yNyw1LjI3LDAsMCwwLDEuMDksMi42NGwtMS4yNSwxLjI1YS42MS42MSwwLDAsMCwwLC44OC42My42MywwLDAsMCwuODgsMGwxLjI1LTEuMjVhNS4yNyw1LjI3LDAsMCwwLDIuNjQsMS4wOVYyNWEuNjIuNjIsMCwwLDAsMS4yNCwwVjIzLjIzYTUuMjcsNS4yNywwLDAsMCwyLjY0LTEuMDlsMS4yNSwxLjI1YS42My42MywwLDAsMCwuODgsMCwuNjEuNjEsMCwwLDAsMC0uODhsLTEuMjUtMS4yNWE1LjI3LDUuMjcsMCwwLDAsMS4wOS0yLjY0SDI1YS42Mi42MiwwLDAsMCwwLTEuMjRabS03LDQuNjhBNC4wNiw0LjA2LDAsMSwxLDIyLjA2LDE4LDQuMDYsNC4wNiwwLDAsMSwxOCwyMi4wNloiLz4KICAgIDxwYXRoIGNsYXNzPSJhNDFjNTIzNC1hMTRhLTRmM2QtOTE2MC00NDcyYjc2ZDAwNDAiIGQ9Ik0yNy45LDI4LjUyYS42Mi42MiwwLDAsMS0uNDQtLjE4LjYxLjYxLDAsMCwxLDAtLjg4LDEzLjQyLDEzLjQyLDAsMCwwLDIuNjMtMTUuMTkuNjEuNjEsMCwwLDEsLjMtLjgzLjYyLjYyLDAsMCwxLC44My4yOSwxNC42NywxNC42NywwLDAsMS0yLjg4LDE2LjYxQS42Mi42MiwwLDAsMSwyNy45LDI4LjUyWiIvPgogICAgPHBhdGggY2xhc3M9ImE0MWM1MjM0LWExNGEtNGYzZC05MTYwLTQ0NzJiNzZkMDA0MCIgZD0iTTI3LjksOC43M2EuNjMuNjMsMCwwLDEtLjQ0LS4xOUExMy40LDEzLjQsMCwwLDAsMTIuMjcsNS45MWEuNjEuNjEsMCwwLDEtLjgzLS4zLjYyLjYyLDAsMCwxLC4yOS0uODNBMTQuNjcsMTQuNjcsMCwwLDEsMjguMzQsNy42NmEuNjMuNjMsMCwwLDEtLjQ0LDEuMDdaIi8+CiAgICA8cGF0aCBjbGFzcz0iYTQxYzUyMzQtYTE0YS00ZjNkLTkxNjAtNDQ3MmI3NmQwMDQwIiBkPSJNNS4zNSwyNC42MmEuNjMuNjMsMCwwLDEtLjU3LS4zNUExNC42NywxNC42NywwLDAsMSw3LjY2LDcuNjZhLjYyLjYyLDAsMCwxLC44OC44OEExMy40MiwxMy40MiwwLDAsMCw1LjkxLDIzLjczYS42MS42MSwwLDAsMS0uMy44M0EuNDguNDgsMCwwLDEsNS4zNSwyNC42MloiLz4KICAgIDxwYXRoIGNsYXNzPSJhNDFjNTIzNC1hMTRhLTRmM2QtOTE2MC00NDcyYjc2ZDAwNDAiIGQ9Ik0xOCwzMi42MkExNC42NCwxNC42NCwwLDAsMSw3LjY2LDI4LjM0YS42My42MywwLDAsMSwwLS44OC42MS42MSwwLDAsMSwuODgsMCwxMy40MiwxMy40MiwwLDAsMCwxNS4xOSwyLjYzLjYxLjYxLDAsMCwxLC44My4zLjYyLjYyLDAsMCwxLS4yOS44M0ExNC42NywxNC42NywwLDAsMSwxOCwzMi42MloiLz4KICAgIDxwYXRoIGNsYXNzPSJhNDFjNTIzNC1hMTRhLTRmM2QtOTE2MC00NDcyYjc2ZDAwNDAiIGQ9Ik0zMCwyOS42MkgyN2EuNjIuNjIsMCwwLDEtLjYyLS42MlYyNmEuNjIuNjIsMCwwLDEsMS4yNCwwdjIuMzhIMzBhLjYyLjYyLDAsMCwxLDAsMS4yNFoiLz4KICAgIDxwYXRoIGNsYXNzPSJhNDFjNTIzNC1hMTRhLTRmM2QtOTE2MC00NDcyYjc2ZDAwNDAiIGQ9Ik03LDMwLjYyQS42Mi42MiwwLDAsMSw2LjM4LDMwVjI3QS42Mi42MiwwLDAsMSw3LDI2LjM4aDNhLjYyLjYyLDAsMCwxLDAsMS4yNEg3LjYyVjMwQS42Mi42MiwwLDAsMSw3LDMwLjYyWiIvPgogICAgPHBhdGggY2xhc3M9ImE0MWM1MjM0LWExNGEtNGYzZC05MTYwLTQ0NzJiNzZkMDA0MCIgZD0iTTI5LDkuNjJIMjZhLjYyLjYyLDAsMCwxLDAtMS4yNGgyLjM4VjZhLjYyLjYyLDAsMCwxLDEuMjQsMFY5QS42Mi42MiwwLDAsMSwyOSw5LjYyWiIvPgogICAgPHBhdGggY2xhc3M9ImE0MWM1MjM0LWExNGEtNGYzZC05MTYwLTQ0NzJiNzZkMDA0MCIgZD0iTTksMTAuNjJBLjYyLjYyLDAsMCwxLDguMzgsMTBWNy42Mkg2QS42Mi42MiwwLDAsMSw2LDYuMzhIOUEuNjIuNjIsMCwwLDEsOS42Miw3djNBLjYyLjYyLDAsMCwxLDksMTAuNjJaIi8+CiAgPC9nPgo8L3N2Zz4K\"/g' bundle/manifests/gatekeeper-operator.clusterserviceversion.yaml
	$(SED) -i 's/mediatype: \"\"/mediatype: \"image\/svg+xml\"/g' bundle/manifests/gatekeeper-operator.clusterserviceversion.yaml
	$(SED) -i 's/^  version:.*/  version: "$(VERSION)"/' bundle/manifests/gatekeeper-operator.clusterserviceversion.yaml
	$(SED) -i '/^    createdAt:.*/d' bundle/manifests/gatekeeper-operator.clusterserviceversion.yaml
	$(SED) -i 's/$(CHANNELS)/"$(CHANNELS)"/g' bundle/metadata/annotations.yaml
	$(SED) -i 's/^    olm.skipRange:.*/    olm.skipRange: "<$(VERSION)"/' bundle/manifests/gatekeeper-operator.clusterserviceversion.yaml
  ifneq ($(REPLACES_VERSION), none)
	  $(SED) -i 's/^  replaces:.*/  replaces: gatekeeper-operator.v$(REPLACES_VERSION)/' bundle/manifests/gatekeeper-operator.clusterserviceversion.yaml
  else
	  $(SED) -i 's/^  replaces:.*/  # replaces: none/' bundle/manifests/gatekeeper-operator.clusterserviceversion.yaml
  endif
	$(OPERATOR_SDK) bundle validate ./bundle

# Requires running cluster (for example through 'make test-cluster')
.PHONY: scorecard
scorecard: bundle
	$(OPERATOR_SDK) scorecard ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(DOCKER) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

OPM = $(LOCAL_BIN)/opm

.PHONY: opm
opm: $(OPM)

$(OPM):
	mkdir -p $(@D)
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$(GOOS)-$(GOARCH)-opm
	chmod +x $(OPM)

# Path used to import Gatekeeper manifests. For example, this could be a local
# file system directory if kustomize has errors using the GitHub URL. See
# https://github.com/kubernetes-sigs/kustomize/issues/4052
IMPORT_MANIFESTS_PATH ?= https://github.com/open-policy-agent/gatekeeper
TMP_IMPORT_MANIFESTS_PATH := $(shell mktemp -d)

# Import Gatekeeper manifests
.PHONY: import-manifests
import-manifests: kustomize
	if [[ $(IMPORT_MANIFESTS_PATH) =~ https://* ]]; then \
		git clone --branch v$(GATEKEEPER_VERSION)  $(IMPORT_MANIFESTS_PATH) $(TMP_IMPORT_MANIFESTS_PATH) ; \
		cd $(TMP_IMPORT_MANIFESTS_PATH) && make patch-image ; \
		$(KUSTOMIZE) build --load-restrictor LoadRestrictionsNone $(TMP_IMPORT_MANIFESTS_PATH)/config/default -o $(MAKEFILE_DIR)/$(GATEKEEPER_MANIFEST_DIR); \
		rm -rf "$${TMP_IMPORT_MANIFESTS_PATH}" ; \
	else \
		$(KUSTOMIZE) build --load-restrictor LoadRestrictionsNone $(IMPORT_MANIFESTS_PATH)/config/default -o $(GATEKEEPER_MANIFEST_DIR); \
	fi

	cd $(GATEKEEPER_MANIFEST_DIR) && $(KUSTOMIZE) edit add resource *.yaml

.PHONY: bundle-index-build
bundle-index-build: opm ## Build the bundle index image.
ifneq ($(REPLACES_VERSION), none)
	$(OPM) index add --bundles $(BUNDLE_IMG) --from-index $(PREV_BUNDLE_INDEX_IMG) --tag $(BUNDLE_INDEX_IMG) -c $(DOCKER)
else
	$(OPM) index add --bundles $(BUNDLE_IMG) --tag $(BUNDLE_INDEX_IMG) -c $(DOCKER)
endif

.PHONY: bundle-index-push
bundle-index-push: ## Push the bundle index image.
	$(MAKE) docker-push IMG=$(BUNDLE_INDEX_IMG)

.PHONY: build-and-push-bundle-images
build-and-push-bundle-images: docker-build docker-push ## Build and push bundle and bundle index images.
	$(MAKE) bundle
	$(MAKE) bundle-build
	$(MAKE) bundle-push
	$(MAKE) bundle-index-build
	$(MAKE) bundle-index-push

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
  FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

# operator-sdk variables
# ======================
OPERATOR_SDK_VERSION ?= v1.31.0
OPERATOR_SDK = $(LOCAL_BIN)/operator-sdk
OPERATOR_SDK_URL := https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$(GOOS)_$(GOARCH)

.PHONY: operator-sdk
operator-sdk: $(OPERATOR_SDK)

$(OPERATOR_SDK):
	# Installing operator-sdk
	mkdir -p $(@D)
	curl -L $(OPERATOR_SDK_URL) -o $(OPERATOR_SDK) || (echo "curl returned $$? trying to fetch operator-sdk"; exit 1)
	chmod +x $(OPERATOR_SDK)

# Default bundle index image tag
BUNDLE_INDEX_IMG ?= $(IMAGE_TAG_BASE)-bundle-index:$(VERSION_TAG)
# Default previous bundle index image tag
PREV_BUNDLE_INDEX_IMG ?= quay.io/gatekeeper/gatekeeper-operator-bundle-index:v$(REPLACES_VERSION)
# Default namespace
NAMESPACE ?= gatekeeper-system
# Default Kubernetes distribution
KUBE_DISTRIBUTION ?= vanilla

MAKEFILE_DIR := $(strip $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST)))))
GATEKEEPER_MANIFEST_DIR ?= config/gatekeeper

# Set version variables for LDFLAGS
GIT_VERSION ?= $(shell git describe --match='v*' --always --dirty)
GIT_HASH ?= $(shell git rev-parse HEAD)
BUILDDATE = $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_TREESTATE = "clean"
DIFF = $(shell git diff --quiet >/dev/null 2>&1; if [ $$? -eq 1 ]; then echo "1"; fi)
ifeq ($(DIFF), 1)
  GIT_TREESTATE = "dirty"
endif

VERSION_PKG = "github.com/gatekeeper/gatekeeper-operator/pkg/version"
LDFLAGS = "-X $(VERSION_PKG).gitVersion=$(GIT_VERSION) \
           -X $(VERSION_PKG).gitCommit=$(GIT_HASH) \
           -X $(VERSION_PKG).gitTreeState=$(GIT_TREESTATE) \
           -X $(VERSION_PKG).buildDate=$(BUILDDATE)"
