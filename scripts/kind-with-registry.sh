#!/usr/bin/env bash
set -o errexit

# desired cluster name; default is "kind"
KIND_NAME="${KIND_NAME:-test-kind}"

reg_name="${KIND_NAME}-registry"
reg_port_default='5000'
REGISTRY_PORT="${REGISTRY_PORT:-${reg_port_default}}"

echo "Checking for running ${reg_name} container..."

# Collect metadata on the registry container
running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" || true)"
reg_current_port="$(docker inspect -f "{{ index .HostConfig.PortBindings \"${reg_port_default}/tcp\" 0 \"HostPort\"}}" "${reg_name}" 2>/dev/null || true)"
reg_container_id="$(docker inspect -f '{{.Id}}' "${reg_name}" 2>/dev/null || true)"

# Stop the container if the ports on the running registry are unexpected
if [ "${running}" == 'true' ] && [[ "${reg_current_port}" != "${REGISTRY_PORT}" ]] && [[ -n "${reg_container_id}" ]]; then
  echo "Stopping misconfigured ${reg_name} container ..."
  docker stop "${reg_container_id}"
fi

# If the registry isn't running or was misconfigured, start a new registry container
if [ "${running}" != 'true' ] || [[ "${reg_current_port}" != "${REGISTRY_PORT}" ]]; then
  if [[ -n "${reg_container_id}" ]]; then
    echo "Removing existing container"
    docker rm "${reg_container_id}"
  fi
  echo "Starting new ${reg_name} container:"
  docker run \
    -d --restart=always -p "127.0.0.1:${REGISTRY_PORT}:${reg_port_default}" --network bridge --name "${reg_name}" \
    registry:2
fi

kind version

KIND_CMD="kind create cluster --name ${KIND_NAME} --wait=5m --config=-"

if [[ ${KIND_CLUSTER_VERSION} != "latest" ]]; then
  KIND_CMD="${KIND_CMD} --image kindest/node:${KIND_CLUSTER_VERSION}"
fi

# create a cluster with the local registry enabled in containerd
cat <<EOF | ${KIND_CMD}
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
EOF

# Add the registry config to the nodes
registry_dir="/etc/containerd/certs.d/localhost:${REGISTRY_PORT}"

for node in $(kind get nodes --name "${KIND_NAME}"); do
  docker exec "${node}" mkdir -p "${registry_dir}"
  cat <<EOF | docker exec -i "${node}" cp /dev/stdin "${registry_dir}/hosts.toml"
[host."http://${reg_name}:${REGISTRY_PORT}"]
EOF
done

# connect the registry to the cluster network
if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${reg_name}")" = 'null' ]; then
  docker network connect kind "${reg_name}"
fi

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
    host: "localhost:${REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
