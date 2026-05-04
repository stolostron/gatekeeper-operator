#! /bin/bash

set -e

# Gatekeeper Operator image
stage_operator_img="quay.io/redhat-user-workloads/gatekeeper-tenant/gatekeeper-operator-3-20/gatekeeper-operator-3-20@sha256:ded7d2592ed6bcb9bebc179b5c73e77951d340349b15698b14c6d1d4c6c3b3ee"
operator_img="registry.redhat.io/gatekeeper/gatekeeper-rhel9-operator@${stage_operator_img##*@}"
# Gatekeeper image
stage_gatekeeper_img="quay.io/redhat-user-workloads/gatekeeper-tenant/gatekeeper-operator-3-20/gatekeeper-3-20@sha256:37d0c2fb5e557434c88270b6d87ea32a27fdcf138eec1e042cf2974e3410292a"
gatekeeper_img="registry.redhat.io/gatekeeper/gatekeeper-rhel9@${stage_gatekeeper_img##*@}"

base_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." >/dev/null 2>&1 && pwd)"

csv_file=${base_dir}/bundle/manifests/gatekeeper-operator-product.clusterserviceversion.yaml

# Patch images in the CSV for:
# Gatekeeper
#   - containerImage annotation
#   - Deployment RELATED_IMAGE_GATEKEEPER env
# Operator
#   - Deployment image
# Both
#   - relatedImages
csv_patch=$(printf '[{
  "op": "replace",
  "path": "/metadata/annotations/containerImage",
  "value": "%s",
},{
  "op": "replace",
  "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/0/env",
  "value": [{
    "name": "RELATED_IMAGE_GATEKEEPER",
    "value": "%s"
  }],
},{
  "op": "replace",
  "path": "/spec/relatedImages",
  "value": [
      { "name":"gatekeeper", "image": "%s" },
      { "name":"gatekeeper-operator", "image": "%s" }
  ],
},{
  "op": "replace",
  "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/0/image",
  "value": "%s",
}]' "${gatekeeper_img}" "${gatekeeper_img}" "${gatekeeper_img}" "${operator_img}" "${operator_img}")

kubectl patch --local=true -f "${csv_file}" --type=json --patch="${csv_patch}" --output=yaml >"${csv_file}.bk"

mv "${csv_file}.bk" "${csv_file}"
