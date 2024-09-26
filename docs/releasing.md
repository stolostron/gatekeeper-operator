# Releasing Gatekeeper Operator

The following steps need to be done in order to successfully automate the
release of the Gatekeeper Operator using the GitHub Actions release workflow.

**NOTE: This assumes that your git remote name for this repository is named
`upstream` and that the remote name for your fork is named `origin`.**

1. Make sure your clone is up-to-date:
    ```shell
    git fetch --prune upstream
    ```
1. Store the current version for use later. If this is the first release in a channel, set this value to `none`.
    ```shell
    RELEASE_PREV_VERSION=$(cat VERSION)
    ```
1. Set the desired version being released:
    ```shell
    RELEASE_VERSION=0.0.1
    ```
1. Checkout a new branch based on `upstream/main`:
    ```shell
    git checkout -b release-$(echo ${RELEASE_VERSION} | cut -d '.' -f 1-2) --no-track upstream/main
    ```
1. Update the version of the operator in the VERSION file:
    ```shell
    printf "${RELEASE_VERSION}" > VERSION
    ```
1. Update the release manifest:
    ```shell
    make release
    ```
1. Update the base CSV `replaces` field. This is **only** needed if the
   previous released version `${RELEASE_PREV_VERSION}` was an official release
   i.e. no release candidate, such that users would have the previous released
   version installed in their cluster via OLM:
    ```shell
    printf "${RELEASE_PREV_VERSION}" > REPLACE_VERSION
    ```
1. Update bundle:
    ```shell
    make bundle
    ```
1. Commit above changes:
    ```shell
    git commit -m "Release ${RELEASE_VERSION}" Makefile ./deploy/ ./bundle/ ./config/manifests/bases/gatekeeper-operator.clusterserviceversion.yaml ./config/manager/kustomization.yaml
    ```
1. Push the changes in the branch to your fork:
    ```shell
    git push -u origin release-${RELEASE_VERSION}
    ```
1. Create a PR with the above changes and merge it. If using the `gh` [GitHub
   CLI](https://cli.github.com/) you can create the PR using:
   ```shell
   gh pr create --repo stolostron/gatekeeper-operator --title "Release ${RELEASE_VERSION}" --body ""
   ```
1. After the PR is merged, fetch the new commits:
    ```shell
    git fetch --prune upstream
    ```
1. Create and push a new release tag. This will trigger the GitHub actions
   release workflow to build and push the release image and create a new
   release on GitHub. Note that `upstream` is used as the remote name for this
   repository:
    ```shell
    git tag -a -m "Release ${RELEASE_VERSION}" ${RELEASE_VERSION} upstream/main
    git push upstream ${RELEASE_VERSION}
    ```
