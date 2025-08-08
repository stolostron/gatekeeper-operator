# Build the manager binary
FROM registry.ci.openshift.org/stolostron/builder:go1.23-linux@sha256:af2180c75966a79c28f24e8876399406d52d44a7ae26f077a0146d136b53833f AS builder

ARG GOOS
ARG GOARCH
ARG LDFLAGS

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=1 GOOS=${GOOS} GOARCH=${GOARCH} go build -mod=readonly -a -ldflags "${LDFLAGS}" -o manager main.go

# Copy to base image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:8d905a93f1392d4a8f7fb906bd49bf540290674b28d82de3536bb4d0898bf9d7
WORKDIR /
COPY --from=builder /workspace/manager .
RUN mkdir licenses/
COPY LICENSE licenses/
USER 65532:65532

ENTRYPOINT ["/manager"]

LABEL name=gatekeeper/gatekeeper-rhel9-operator
LABEL summary="Red Hat Gatekeeper Operator"
LABEL description="The Gatekeeper Operator installs and configures Open Policy Agent Gatekeeper."
LABEL version=v3.18.1
LABEL maintainer="acm-component-maintainers@redhat.com"
LABEL com.redhat.component=gatekeeper-operator-container
LABEL io.openshift.tags="data,images"
LABEL io.k8s.display-name="Gatekeeper Operator"
LABEL io.k8s.description="The Gatekeeper Operator installs and configures Open Policy Agent Gatekeeper."
