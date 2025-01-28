#! /bin/bash

set -e

# Gatekeeper Operator image
operator_img="quay.io/redhat-user-workloads/gatekeeper-tenant/gatekeeper-operator-3-17/gatekeeper-3-17@sha256:4afb0caa019880603a8603934808233222a93fd334d14a3acad23750e39c3834"
# Gatekeeper image
gatekeeper_img="quay.io/redhat-user-workloads/gatekeeper-tenant/gatekeeper-operator-3-17/gatekeeper-operator-3-17@sha256:4fe9c380a0660c30214383395f5bebacb2e1fc381e4fc7021ef93eac0728cde3"

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
}]' ${gatekeeper_img} ${gatekeeper_img} ${gatekeeper_img} ${operator_img} ${operator_img})

kubectl patch --local=true -f ${csv_file} --type=json --patch="${csv_patch}" --output=yaml >${csv_file}.bk

mv ${csv_file}.bk ${csv_file}
