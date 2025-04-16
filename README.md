# Gatekeeper Operator

[![CI Tests](https://github.com/stolostron/gatekeeper-operator/actions/workflows/ci_tests.yaml/badge.svg)](https://github.com/stolostron/gatekeeper-operator/actions/workflows/ci_tests.yaml)
[![OLM Tests](https://github.com/stolostron/gatekeeper-operator/actions/workflows/olm_tests.yaml/badge.svg)](https://github.com/stolostron/gatekeeper-operator/actions/workflows/olm_tests.yaml)
[![Create Release](https://github.com/stolostron/gatekeeper-operator/actions/workflows/release.yaml/badge.svg)](https://github.com/stolostron/gatekeeper-operator/actions/workflows/release.yaml)
[![Docker Repository on Quay](https://img.shields.io/:Image-Quay-blue.svg)](https://quay.io/repository/gatekeeper/gatekeeper-operator)

The Gatekeeper Operator installs the Open Policy Agent (OPA) Gatekeeper using the Operator Lifecycle
Manager (OLM). It references the [`stolostron/gatekeeper`](https://github.com/stolostron/gatekeeper)
repository, a fork of the upstream `open-policy-agent/gatekeeper`.

**NOTE**: View the `release-X.Y` branches (or associated tags) for relevant releases from
`stolostron/gatekeeper-operator`.

**References:**

- [Open Policy Agent (OPA) Gatekeeper](https://open-policy-agent.github.io/gatekeeper/website/)
- [Operator Lifecycle Manager (OLM)](https://olm.operatorframework.io/)
