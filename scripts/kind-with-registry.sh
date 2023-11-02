#!/usr/bin/env bash
set -o errexit

# desired cluster name; default is "kind"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kind}"

# create registry container unless it's already running
reg_name='kind-registry'
reg_port="${REGISTRY_PORT:-5000}"
echo "Checking for running ${reg_name} container..."
running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" || true)"
if [ "${running}" != 'true' ]; then
  REG_CONTAINER_ID="$(docker inspect -f '{{.Id}}' "${reg_name}" || true)"
  if [[ -n "${REG_CONTAINER_ID}" ]]; then
    echo "Removing existing container:"
    docker rm ${REG_CONTAINER_ID}
  fi
  echo "Starting new ${reg_name} container:"
  docker run \
    -d --restart=always -p "${reg_port}:5000" --name "${reg_name}" \
    registry:2
fi

kind version

KIND_CMD=
if [[ -z "${KIND_CLUSTER_VERSION}" ]]; then
  KIND_CMD="kind create cluster --name ${KIND_CLUSTER_NAME} --wait=5m --config=-"
else
  KIND_CMD="kind create cluster --image kindest/node:${KIND_CLUSTER_VERSION} --name ${KIND_CLUSTER_NAME} --wait=5m --config=-"
fi

# create a cluster with the local registry enabled in containerd
cat <<EOF | ${KIND_CMD}
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_name}:${reg_port}"]
EOF

# connect the registry to the cluster network
docker network connect "kind" "${reg_name}" || true

# Document the local registry
# https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${reg_port}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
