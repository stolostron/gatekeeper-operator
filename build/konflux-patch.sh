#! /bin/bash

set -e

# Gatekeeper Operator image
operator_img="registry.redhat.io/gatekeeper/gatekeeper-rhel9-operator@sha256:6e386be134d928bdb03b702e399c97e7aedecacaa3d0813183a8c5ecf13c7bc2"
# Gatekeeper image
gatekeeper_img="registry.redhat.io/gatekeeper/gatekeeper-rhel9@sha256:3095f68c12c5dc3b00ce84e1c37d516d96cbcb06d42eaef5372358786956bd62"

build_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

csv_file=${build_dir}/../bundle/manifests/gatekeeper-operator.clusterserviceversion.yaml

csv_patch=$(printf '[{
  "op": "replace",
  "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/0/env/0/value",
  "value": "%s",
},{
  "op": "replace",
  "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/0/image",
  "value": "%s",
},{
  "op": "replace",
  "path": "/spec/relatedImages/0/image",
  "value": "%s",
}]' ${gatekeeper_img} ${operator_img} ${gatekeeper_img})

kubectl patch --local=true -f ${csv_file} --type=json --patch="${csv_patch}" --output=yaml >${csv_file}.bk
mv ${csv_file}.bk ${csv_file}
