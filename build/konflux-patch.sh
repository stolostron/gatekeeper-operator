#! /bin/bash

set -e

# Gatekeeper Operator image
stage_operator_img="quay.io/redhat-user-workloads/gatekeeper-tenant/gatekeeper-operator-3-17/gatekeeper-operator-3-17@sha256:2bb630b5bd7b03369d565897ed2626dfee174a35f560df13a1c6dc4852943b2c"
operator_img="registry.redhat.io/gatekeeper/gatekeeper-rhel9-operator@${stage_operator_img##*@}"
# Gatekeeper image
stage_gatekeeper_img="quay.io/redhat-user-workloads/gatekeeper-tenant/gatekeeper-operator-3-17/gatekeeper-3-17@sha256:727f47e0b429f02391feabc84b23a06c421cb900dff414e87d78c7d9f80dc2db"
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
