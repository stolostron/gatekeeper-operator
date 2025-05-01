## CLI versions (with links to the latest releases)
# https://github.com/operator-framework/operator-registry/releases/latest
OPM_VERSION ?= v1.39.0
# https://github.com/go-bindata/go-bindata/releases/latest
GO_BINDATA_VERSION ?= v3.1.3
# https://github.com/operator-framework/operator-lifecycle-manager/releases/latest
OLM_VERSION ?= v0.27.0
# https://github.com/operator-framework/operator-sdk/releases/latest
OPERATOR_SDK_VERSION ?= v1.34.1
# https://github.com/kubernetes/kubernetes/releases/latest
KUBERNETES_VERSION ?= v1.29.4
# https://github.com/bats-core/bats-core/releases/latest
BATS_VERSION ?= 1.11.0

# Versioning and replacement version for the operator bundle.
# (Stored in files of the same name at the base of the repo.)
VERSION ?= $(shell cat VERSION)
REPLACES_VERSION ?= $(shell cat REPLACES_VERSION)
# Version of the underlying Gatekeeper--defaults to the version of the operator.
# (Can be overridden by creating a GATEKEEPER_VERSION file at the base of the repo.)
GATEKEEPER_VERSION ?= $(shell cat GATEKEEPER_VERSION 2>/dev/null || cat VERSION)
PROJECT_NAME ?= $(shell yq '.projectName' PROJECT)

# CHANNELS define the bundle channels used in the bundle.
CHANNELS ?= $(shell echo $(VERSION) | cut -d '.' -f 1-2)
ifneq ($(origin CHANNELS), undefined)
  BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
DEFAULT_CHANNEL ?= "3.15"
ifneq ($(origin DEFAULT_CHANNEL), undefined)
  BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif

# IMAGE_TAG_BASE defines the image registry namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
VERSION_TAG ?= v$(VERSION)
REPO ?= quay.io/gatekeeper
IMAGE_TAG_BASE ?= $(REPO)/gatekeeper-operator
IMG ?= $(IMAGE_TAG_BASE):$(VERSION_TAG)

BUNDLE_IMG_BASE ?= $(IMAGE_TAG_BASE)-bundle
BUNDLE_IMG ?= $(BUNDLE_IMG_BASE):$(VERSION_TAG)

BUNDLE_INDEX_IMG ?= $(BUNDLE_IMG_BASE)-index:$(VERSION_TAG)

# Default deployment namespace for Gatekeeper
NAMESPACE ?= gatekeeper-system

LOCAL_BIN ?= $(PWD)/ci-tools/bin
export PATH := $(LOCAL_BIN):$(PATH)
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
MAKEFILE_DIR := $(strip $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST)))))
# Option to use podman or docker
DOCKER ?= docker
ifeq ($(DOCKER),podman)
	TLS_VERIFY = --tls-verify=false
	USE_HTTP = --use-http
endif

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
  GOBIN=$(shell go env GOPATH)/bin
else
  GOBIN=$(shell go env GOBIN)
endif

# Fix sed issues on mac by using GSED and fix base64 issues on macos by omitting the -w 0 parameter
SED = sed
BASE64 = base64 -w 0
ifeq ($(GOOS), darwin)
  SED = gsed
	BASE64 = base64
endif

include build/common/Makefile.common.mk

.PHONY: all
all: build

############################################################
##@ General
############################################################

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

BATS := $(LOCAL_BIN)/bats

.PHONY: download-binaries
download-binaries: kustomize go-bindata envtest controller-gen
	# Checking installation of bats v$(BATS_VERSION)
	@if [ ! -f $(BATS) ] || [ "$(shell $(BATS) --version)" != "Bats $(BATS_VERSION)" ]; then \
		echo "Downloading and installing bats"; \
		curl -sSLO https://github.com/bats-core/bats-core/archive/v${BATS_VERSION}.tar.gz; \
		tar -zxf v${BATS_VERSION}.tar.gz; \
		bash bats-core-${BATS_VERSION}/install.sh $(PWD)/ci-tools; \
		rm -rf bats-core-${BATS_VERSION} v${BATS_VERSION}.tar.gz; \
	fi

############################################################
##@ Build
############################################################

# Targets to install binaries are defined in build/common/Makefile.common.mk
.PHONY: controller-gen
controller-gen:

.PHONY: kustomize
kustomize:

.PHONY: envtest
envtest:

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases output:rbac:dir=config/rbac

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Path used to import Gatekeeper manifests
IMPORT_MANIFESTS_PATH ?= https://github.com/stolostron/gatekeeper
TMP_IMPORT_MANIFESTS_PATH := $(shell mktemp -d)
GATEKEEPER_MANIFEST_DIR ?= config/gatekeeper

GATEKEEPER_TAG := $(shell curl -sL https://api.github.com/repos/stolostron/gatekeeper/tags | jq -r '.[].name' | sort --version-sort | grep $(shell echo $(GATEKEEPER_VERSION) | cut -d '.' -f 1-2) | tail -1 )

.PHONY: import-manifests
import-manifests: kustomize ## Import Gatekeeper manifests.
	if [ "$(shell echo $(GATEKEEPER_TAG) | cut -d '-' -f 1)" != "v$(GATEKEEPER_VERSION)" ]; then \
		echo "error: Gatekeeper version v$(GATEKEEPER_VERSION) set in the operator doesn't match Gatekeeper tag $(shell echo $(GATEKEEPER_TAG) | cut -d '-' -f 1) from $(IMPORT_MANIFESTS_PATH)." ;\
		exit 1 ;\
	fi 
	if [ "$${IMPORT_MANIFESTS_PATH#https://}" != "$(IMPORT_MANIFESTS_PATH)" ]; then \
		git clone --depth=1 --branch $(GATEKEEPER_TAG) $(IMPORT_MANIFESTS_PATH) $(TMP_IMPORT_MANIFESTS_PATH) ; \
		cd $(TMP_IMPORT_MANIFESTS_PATH) && make patch-image ; \
		$(KUSTOMIZE) build --load-restrictor LoadRestrictionsNone $(TMP_IMPORT_MANIFESTS_PATH)/config/default -o $(MAKEFILE_DIR)/$(GATEKEEPER_MANIFEST_DIR); \
		rm -rf $(TMP_IMPORT_MANIFESTS_PATH) ; \
	else \
		$(KUSTOMIZE) build --load-restrictor LoadRestrictionsNone $(IMPORT_MANIFESTS_PATH)/config/default -o $(MAKEFILE_DIR)/$(GATEKEEPER_MANIFEST_DIR); \
	fi
	cd $(MAKEFILE_DIR)/$(GATEKEEPER_MANIFEST_DIR) && $(KUSTOMIZE) edit add resource *.yaml

GO_BINDATA = $(LOCAL_BIN)/go-bindata
BINDATA_OUTPUT_FILE := pkg/bindata/bindata.go

.PHONY: go-bindata
go-bindata:
	$(call go-get-tool,github.com/go-bindata/go-bindata/v3/go-bindata@${GO_BINDATA_VERSION})

.PHONY: update-bindata
update-bindata: go-bindata ## Update bindata.go file.
	mkdir -p ./$(GATEKEEPER_MANIFEST_DIR)-rendered
	$(KUSTOMIZE) build $(GATEKEEPER_MANIFEST_DIR) -o ./$(GATEKEEPER_MANIFEST_DIR)-rendered
	$(GO_BINDATA) -nocompress -nometadata \
		-prefix "bindata" \
		-pkg "bindata" \
		-o "$(BINDATA_OUTPUT_FILE)" \
		-ignore "OWNERS" \
		./$(GATEKEEPER_MANIFEST_DIR)-rendered/...
	rm -rf ./$(GATEKEEPER_MANIFEST_DIR)-rendered
	$(MAKE) fmt

GATEKEEPER_IMAGE ?= quay.io/gatekeeper/gatekeeper

.PHONY: update-gatekeeper-image
update-gatekeeper-image: ## Update Gatekeeper image in manifests.
	yq 'select(.kind == "Deployment") \
		|= .spec.template.spec.containers[] \
		|= select(.name == "manager").env[] \
		|= select(.name == "RELATED_IMAGE_GATEKEEPER").value = "$(GATEKEEPER_IMAGE):v$(GATEKEEPER_VERSION)"' \
		-i config/manager/manager.yaml

# Set version variables for LDFLAGS
GIT_VERSION ?= $(shell git describe --match='v*' --always --dirty)
GIT_HASH ?= $(shell git rev-parse HEAD)
BUILDDATE = $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_TREESTATE = "clean"
DIFF = $(shell git diff --quiet >/dev/null 2>&1; if [ $$? -eq 1 ]; then echo "1"; fi)
ifeq ($(DIFF), 1)
  GIT_TREESTATE = "dirty"
endif

VERSION_PKG = "github.com/stolostron/gatekeeper-operator/pkg/version"
LDFLAGS = "-X $(VERSION_PKG).gitVersion=$(GIT_VERSION) \
           -X $(VERSION_PKG).gitCommit=$(GIT_HASH) \
           -X $(VERSION_PKG).gitTreeState=$(GIT_TREESTATE) \
           -X $(VERSION_PKG).buildDate=$(BUILDDATE)"

.PHONY: build
build: generate fmt vet ## Build manager binary.
	CGO_ENABLED=1 GOFLAGS=$(GOFLAGS) go build -ldflags $(LDFLAGS) -o bin/manager main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	$(DOCKER) build --platform linux/$(GOARCH) --build-arg GOOS=linux --build-arg GOARCH=$(GOARCH) --build-arg LDFLAGS=${LDFLAGS} -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(DOCKER) push ${IMG} $(TLS_VERIFY)

.PHONY: release
release: manifests kustomize patch-deployment
	$(KUSTOMIZE) build config/default > ./deploy/gatekeeper-operator.yaml

############################################################
##@ Deploy
############################################################

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host using the currently configured Kubernetes cluster.
	GOFLAGS=$(GOFLAGS) GATEKEEPER_TARGET_NAMESPACE=$(NAMESPACE) go run -ldflags $(LDFLAGS) ./main.go

.PHONY: install
install: manifests kustomize ## Install CRDs into the currently configured K8s cluster.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the currently configured K8s cluster.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: manifests kustomize patch-deployment apply-manifests unpatch-deployment ## Deploy controller using the currently configured Kubernetes cluster.

.PHONY: apply-manifests
apply-manifests:
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: patch-deployment
patch-deployment:
	cd config/default && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)

.PHONY: unpatch-deployment
unpatch-deployment: 
	cd config/default && $(KUSTOMIZE) edit set namespace gatekeeper-system
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMAGE_TAG_BASE):$(VERSION_TAG)

.PHONY: undeploy
undeploy: ## Undeploy controller from the the currently configured Kubernetes cluster.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

.PHONY: deploy-ci
deploy-ci: install patch-image deploy unpatch-image ## Deploys the controller with a patched pull policy.

.PHONY: deploy-olm
deploy-olm: operator-sdk
	$(OPERATOR_SDK) olm install --version $(OLM_VERSION) --timeout 5m

.PHONY: deploy-using-olm
deploy-using-olm:
	$(SED) -i 's#quay.io/gatekeeper/gatekeeper-operator-bundle-index:latest#$(BUNDLE_INDEX_IMG)#g' config/olm-install/kustomization.yaml
	$(SED) -i 's#channel: stable#channel: $(DEFAULT_CHANNEL)#g' config/olm-install/kustomization.yaml
	cd config/olm-install && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
	$(KUSTOMIZE) build config/olm-install | kubectl apply -f -

.PHONY: patch-image
patch-image:
	$(SED) -i 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' config/manager/manager.yaml

.PHONY: unpatch-image
unpatch-image:
	$(SED) -i 's/imagePullPolicy: IfNotPresent/imagePullPolicy: Always/g' config/manager/manager.yaml

############################################################
##@ Operator Bundling
############################################################

OPERATOR_SDK = $(LOCAL_BIN)/operator-sdk
OPERATOR_SDK_URL := https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$(GOOS)_$(GOARCH)

$(OPERATOR_SDK):
	mkdir -p $(@D)

.PHONY: operator-sdk
operator-sdk: $(OPERATOR_SDK)
	# Checking installation of operator-sdk $(OPERATOR_SDK_VERSION)
	@if [ "$$($(OPERATOR_SDK) version 2>/dev/null | grep -o "v[0-9]\+\.[0-9]\+\.[0-9]\+" | head -1)" != "$(OPERATOR_SDK_VERSION)" ]; then \
		echo "Installing operator-sdk"; \
		curl -L $(OPERATOR_SDK_URL) -o $(OPERATOR_SDK) || (echo "curl returned $$? trying to fetch operator-sdk"; exit 1); \
		chmod +x $(OPERATOR_SDK); \
	fi

.PHONY: bundle
bundle: operator-sdk manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --manifests --overwrite --version $(VERSION) $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)
	# Set base64data in CSV with SVG logo: $(SED) -i 's/base64data: ""/base64data: "<base64-string>"/g' bundle/manifests/$(PROJECT_NAME).clusterserviceversion.yaml 
	@$(SED) -i 's/base64data: \"\"/base64data: \"$(shell $(BASE64) -i bundle/logo.svg)\"/g' bundle/manifests/$(PROJECT_NAME).clusterserviceversion.yaml
	$(SED) -i 's/mediatype: \"\"/mediatype: \"image\/svg+xml\"/g' bundle/manifests/$(PROJECT_NAME).clusterserviceversion.yaml
	$(SED) -i 's/^  version:.*/  version: "$(VERSION)"/' bundle/manifests/$(PROJECT_NAME).clusterserviceversion.yaml
	$(SED) -i '/^    createdAt:.*/d' bundle/manifests/$(PROJECT_NAME).clusterserviceversion.yaml
	yq '.annotations["operators.operatorframework.io.bundle.channels.v1"] = "$(CHANNELS)"' -i bundle/metadata/annotations.yaml
	yq '.annotations.version = "v$(VERSION)"' -i bundle/metadata/annotations.yaml
	$(SED) -i 's/^    olm.skipRange:.*/    olm.skipRange: "<$(VERSION)"/' bundle/manifests/$(PROJECT_NAME).clusterserviceversion.yaml
  ifneq ($(REPLACES_VERSION), none)
	  $(SED) -i 's/^  replaces:.*/  replaces: $(PROJECT_NAME).v$(REPLACES_VERSION)/' bundle/manifests/$(PROJECT_NAME).clusterserviceversion.yaml
  else
	  $(SED) -i 's/^  replaces:.*/  # replaces: none/' bundle/manifests/$(PROJECT_NAME).clusterserviceversion.yaml
  endif
	@for bundle in build/bundle.Dockerfile*; do \
		awk '/FROM/,/# Core bundle annotations/' $${bundle} | sed '$$d' > $${bundle}.tmp; \
		mv $${bundle}.tmp $${bundle}; \
		yq '.annotations' bundle/metadata/annotations.yaml | sed 's/: /=/' | sed 's/^\([^#]\)/LABEL \1/' >> $${bundle}; \
	done
	$(OPERATOR_SDK) bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(DOCKER) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

OPM = $(LOCAL_BIN)/opm

$(OPM):
	mkdir -p $(@D)

.PHONY: opm
opm: $(OPM)
	# Checking installation of opm $(OPM_VERSION)
	@if [ "$$($(OPM) version 2>/dev/null | grep -o "v[0-9]\+\.[0-9]\+\.[0-9]\+" | head -1)" != "$(OPM_VERSION)" ]; then \
		echo "Installing opm"; \
		curl -Lo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$(GOOS)-$(GOARCH)-opm; \
		chmod +x $(OPM); \
	fi

.PHONY: bundle-index-build
bundle-index-build: opm ## Build the bundle index image.
ifneq ($(REPLACES_VERSION), none)
	$(OPM) index add --bundles $(BUNDLE_IMG) --from-index quay.io/gatekeeper/gatekeeper-operator-bundle-index:v$(REPLACES_VERSION) --tag $(BUNDLE_INDEX_IMG) -c $(DOCKER) $(USE_HTTP)
else
	$(OPM) index add --bundles $(BUNDLE_IMG) --tag $(BUNDLE_INDEX_IMG) -c $(DOCKER) $(USE_HTTP)
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

############################################################
##@ Lint and Test
############################################################

# Targets to lint code are defined in build/common/Makefile.common.mk
.PHONY: fmt
fmt: ## Run go fmt against code.

.PHONY: lint
lint: ## Run golangci-lint against code.

.PHONY: vet
vet: ## Run go vet against code.
	GOFLAGS=$(GOFLAGS) go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest test-unit ## Run tests.

.PHONY: test-unit
test-unit:
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(KUBERNETES_VERSION:v%=%) -p path)" GOFLAGS=$(GOFLAGS) go test $(TESTARGS) $$(go list ./... | grep -v /test/)

.PHONY: test-coverage
test-coverage: TESTARGS = -json -cover -covermode=atomic -coverprofile=coverage_unit.out
test-coverage: test-unit

E2E_LABEL_FILTER = --label-filter="!openshift"
.PHONY: test-e2e
test-e2e: e2e-dependencies generate fmt vet ## Run e2e tests using the configured Kubernetes cluster.
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
test-cluster: ## Create a local kind cluster with a registry for testing.
	KIND_CLUSTER_VERSION=$(KUBERNETES_VERSION) ./scripts/kind-with-registry.sh

.PHONY: kind-load-image
kind-load-image:
	kind load docker-image $(IMG) --name $(KIND_NAME)

.PHONY: kind-bootstrap-cluster
kind-bootstrap-cluster: test-cluster install dev-build kind-load-image
	kubectl label ns $(NAMESPACE)  --overwrite pod-security.kubernetes.io/audit=privileged
	kubectl label ns $(NAMESPACE)  --overwrite pod-security.kubernetes.io/enforce=privileged
	kubectl label ns $(NAMESPACE)  --overwrite pod-security.kubernetes.io/warn=privileged
	$(MAKE) deploy-ci NAMESPACE=$(NAMESPACE) IMG=$(IMG)
	kubectl -n $(NAMESPACE) wait deployment/gatekeeper-operator-controller --for condition=Available --timeout=90s

CLUSTER_NAME = kind
KIND_NAME ?= test-kind
.PHONY: delete-test-cluster
delete-test-cluster: ## Clean up the local kind cluster and registry.
	# Stopping and removing the registry container
	-docker stop $(shell docker inspect -f '{{.Id}}' kind-registry 2>/dev/null || printf "-")
	-docker rm $(shell docker inspect -f '{{.Id}}' kind-registry 2>/dev/null || printf "-")
	-kind delete cluster --name "$(KIND_NAME)"

# Requires running cluster (for example through 'make test-cluster')
.PHONY: scorecard
scorecard: bundle
	$(OPERATOR_SDK) scorecard ./bundle

.PHONY: test-gatekeeper-e2e
test-gatekeeper-e2e:
	kubectl -n $(NAMESPACE) apply -f ./config/samples/gatekeeper_e2e_test.yaml
	bats --version

############################################################
##@ Targets from build/common/Makefile.common.mk
############################################################
