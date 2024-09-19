# Releasing Gatekeeper Operator

The following steps need to be done in order to successfully automate the release of the Gatekeeper
Operator using the GitHub Actions release workflow.

1. Check the latest upstream release
   https://github.com/open-policy-agent/gatekeeper/releases/latest. Ensure an associated release
   branch has been created and updated in https://github.com/stolostron/gatekeeper and tagged with
   the correct version.
2. Make sure your clone is up-to-date and check out the `main` branch:
   ```shell
   STOLOSTRON="$(git remote -v | grep push | awk '/stolostron/ {print $1}')"
   git fetch --prune ${STOLOSTRON}
   git checkout ${STOLOSTRON}/main
   ```
3. Update the `VERSION` file with the latest operator version, and then store the latest released
   version for use later:
   ```shell
   RELEASE_PREV_VERSION=$(cat VERSION)
   ```
4. Set the desired operator version being released:
   ```shell
   RELEASE_VERSION=1.2.3
   ```
5. Remove any `GATEKEEPER_VERSION` file:
   ```shell
   rm GATEKEEPER_VERSION
   ```
   If the latest upstream Gatekeeper version is a different z-version than the current operator
   version, instantiate a new Gatekeeper version file with the differing version:
   ```shell
   printf "1.2.3" > GATEKEEPER_VERSION
   ```
6. Update the `go.mod` with the matching Gatekeeper version:
   ```shell
   GATEKEEPER_VERSION=$(cat GATEKEEPER_VERSION 2>/dev/null || cat VERSION)
   go get github.com/open-policy-agent/gatekeeper/v3@v${GATEKEEPER_VERSION}
   go mod tidy
   ```
7. Checkout a new branch based on `${STOLOSTRON}/main`:
   ```shell
   git checkout -b create-release-$(echo ${RELEASE_VERSION})
   ```
8. Update the version of the operator in the `VERSION` file, and update the base CSV `replaces`
   field:
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
   **NOTE**: If this is a z-stream release for a previous y-stream and there is a subsequent
   y-stream released, then `CHANNEL` and `DEFAULT_CHANNEL` must be set to remove the `stable`
   channel prior to running the release Make targets:
   ```shell
   export CHANNEL=$(cat VERSION | cut -d '.' -f 1-2)
   export DEFAULT_CHANNEL=${CHANNEL}
   ```
10. Check whether Gatekeeper permissions have changed. If they have, they'll need to also be updated
    in [`controllers/gatekeeper_controller.go`](../controllers/gatekeeper_controller.go#L170) so
    that the operator can grant those permissions:
    ```shell
    git diff config/gatekeeper/rbac.authorization.k8s.io_v1_*
    ```
    If permissions are updated, you need to run these commands to update the appropriate manifests:
    ```shell
    make manifests
    make release
    make bundle
    ```
11. Check for and fix any linting errors:
    ```shell
    make fmt
    make lint
    ```
12. Commit above changes:
    ```shell
    git commit --signoff -am "Release ${RELEASE_VERSION}"
    ```
13. Push the changes in the branch to your fork (set `FORK` manually if you have more than one
    development fork):
    ```shell
    FORK="$(git remote -v | grep push | awk '!/stolostron/ {print $1}')"
    git push -u ${FORK} create-release-${RELEASE_VERSION}
    ```
14. Create a PR with the above changes and merge it. If using the `gh`
    [GitHub CLI](https://cli.github.com/) you can create the PR using:
    ```shell
    gh pr create --repo stolostron/gatekeeper-operator --title "Release ${RELEASE_VERSION}" --body ""
    ```
15. After the PR is merged (and any subsequent fixes), fetch the new commits:
    ```shell
    STOLOSTRON="$(git remote -v | grep push | awk '/stolostron/ {print $1}')"
    git fetch --prune ${STOLOSTRON}
    ```
16. Create and push a new release tag. This will trigger the GitHub actions release workflow to
    build and push the release image and create a new release on GitHub:
    ```shell
    RELEASE_VERSION="$(cat VERSION)"
    git tag -a ${RELEASE_VERSION} -m "${RELEASE_VERSION}" ${STOLOSTRON}/main
    git push ${STOLOSTRON} ${RELEASE_VERSION}
    ```
