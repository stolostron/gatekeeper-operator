FROM scratch

# Copy files to locations specified by labels.
COPY bundle/manifests /manifests/
COPY bundle/metadata /metadata/

# Core bundle annotations.
LABEL operators.operatorframework.io.bundle.channel.default.v1=stable
LABEL operators.operatorframework.io.bundle.channels.v1="stable,3.17"
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=gatekeeper-operator
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.34.1
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v3
# Red Hat annotations.
LABEL com.redhat.component=gatekeeper-operator-bundle-container
LABEL com.redhat.delivery.backport=false
LABEL com.redhat.delivery.operator.bundle=true
LABEL com.redhat.openshift.versions=v4.12
# Bundle metadata
LABEL name=gatekeeper/gatekeeper-operator-bundle
LABEL description=gatekeeper-operator-bundle
LABEL io.k8s.description=gatekeeper-operator-bundle
LABEL summary="The Gatekeeper Operator installs and configures Open Policy Agent Gatekeeper."
LABEL version=v3.17.0
LABEL release=""
LABEL distribution-scope=public
LABEL maintainer="acm-component-maintainers@redhat.com"
LABEL url=https://github.com/stolostron/gatekeeper-operator
LABEL vendor="Red Hat"
