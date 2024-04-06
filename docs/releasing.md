# Releasing Gatekeeper Operator

The following steps need to be done in order to successfully automate the release of the Gatekeeper Operator using the
GitHub Actions release workflow.

**NOTE: This assumes that your git remote name for this repository is named `upstream` and that the remote name for your
fork is named `origin`.**

1. Check the latest upstream release https://github.com/open-policy-agent/gatekeeper/releases/latest. Ensure an
   associated release branch has been created and updated in https://github.com/stolostron/gatekeeper and tagged with
   the correct version.
2. Make sure your clone is up-to-date and check out the `main` branch (this assumes your local remote of
   https://github.com/stolostron/gatekeeper-operator is called `upstream`):
   ```shell
   git fetch --prune upstream
   git checkout upstream/main
   ```
3. Update the `VERSION` file with the latest operator version, and then store the latest released version for use later:
   ```shell
   RELEASE_PREV_VERSION=$(cat VERSION)
   ```
4. Set the desired operator version being released:
   ```shell
   RELEASE_VERSION=1.2.3
   ```
5. If the latest upstream Gatekeeper version is a different z-version than the current operator version, set the
   Gatekeeper version:
   ```shell
   printf "1.2.3" > GATEKEEPER_VERSION
   ```
6. Update the `go.mod` with the matching Gatekeeper version:
   ```shell
   GATEKEEPER_VERSION=$(cat GATEKEEPER_VERSION 2>/dev/null || cat VERSION)
   go get github.com/open-policy-agent/gatekeeper/v3@v${GATEKEEPER_VERSION}
   go mod tidy
   ```
7. Checkout a new branch based on `upstream/main`:
   ```shell
   git checkout -b create-release-$(echo ${RELEASE_VERSION})
   ```
8. Update the version of the operator in the `VERSION` file, and update the base CSV `replaces` field:
   ```shell
   printf "${RELEASE_VERSION}" > VERSION
   printf "${RELEASE_PREV_VERSION}" > REPLACES_VERSION
   ```
9. Update the release manifest and update the bundle:
   ```shell
   make update-gatekeeper-image
   make import-manifests
   make update-bindata
   make release
   make bundle
   ```
   **NOTE**: If this is a z-stream release for a previous y-stream and there is a subsequent y-stream released, then
   `CHANNEL` and `DEFAULT_CHANNEL` must be set to remove the `stable` channel prior to running the release Make targets:
   ```shell
   export CHANNEL=$(cat VERSION | cut -d '.' -f 1-2)
   export DEFAULT_CHANNEL=${CHANNEL}
   ```
10. Commit above changes:
    ```shell
    git commit --signoff -am "Release ${RELEASE_VERSION}"
    ```
11. Push the changes in the branch to your fork:
    ```shell
    git push -u origin create-release-${RELEASE_VERSION}
    ```
12. Create a PR with the above changes and merge it. If using the `gh` [GitHub CLI](https://cli.github.com/) you can
    create the PR using:
    ```shell
    gh pr create --repo stolostron/gatekeeper-operator --title "Release ${RELEASE_VERSION}" --body ""
    ```
13. After the PR is merged (and any subsequent fixes), fetch the new commits:
    ```shell
    git fetch --prune upstream
    ```
14. Create and push a new release tag. This will trigger the GitHub actions release workflow to build and push the
    release image and create a new release on GitHub. Note that `upstream` is used as the remote name for this
    repository:
    ```shell
    RELEASE_VERSION="$(cat VERSION)"
    git tag -a ${RELEASE_VERSION} -m "${RELEASE_VERSION}" upstream/main
    git push upstream ${RELEASE_VERSION}
    ```
