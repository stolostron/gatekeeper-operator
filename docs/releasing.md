# Releasing Gatekeeper Operator

The following steps need to be done in order to successfully automate the release of the Gatekeeper Operator using the
GitHub Actions release workflow.

**NOTE: This assumes that your git remote name for this repository is named `upstream` and that the remote name for your
fork is named `origin`.**

1. Enable [fast-forwarding](https://github.com/stolostron/gatekeeper-operator/actions/workflows/fast_forward.yaml) to
   push commits to a new release branch.
2. Make sure your clone is up-to-date:
   ```shell
   git fetch --prune upstream
   ```
3. Store the current version for use later. If this is the first release in a channel, set this value to `none`.
   ```shell
   RELEASE_PREV_VERSION=$(cat VERSION)
   ```
4. Set the desired version being released:
   ```shell
   RELEASE_VERSION=1.2.3
   ```
5. Check the latest upstream Gatekeeper. If it's a different z-version than the current operator version, set the
   version:
   ```shell
   printf "1.2.3" > GATEKEEPER_VERSION
   ```
6. Checkout a new branch based on `upstream/main`:
   ```shell
   git checkout -b create-release-$(echo ${RELEASE_VERSION}) --no-track upstream/main
   ```
7. Update the version of the operator in the VERSION file:
   ```shell
   printf "${RELEASE_VERSION}" > VERSION
   ```
8. Update the release manifest:
   ```shell
   make release
   ```
9. Update the base CSV `replaces` field. This is **only** needed if the previous released version
   `${RELEASE_PREV_VERSION}` was an official release i.e. no release candidate, such that users would have the previous
   released version installed in their cluster via OLM:
   ```shell
   printf "${RELEASE_PREV_VERSION}" > REPLACES_VERSION
   ```
10. Update bundle:

    ```shell
    make bundle
    ```

11. Commit above changes:

    ```shell
    git commit -am "Release ${RELEASE_VERSION}"
    ```

12. Push the changes in the branch to your fork:

    ```shell
    git push -u origin create-release-${RELEASE_VERSION}
    ```

13. Create a PR with the above changes and merge it. If using the `gh` [GitHub CLI](https://cli.github.com/) you can
    create the PR using:

    ```shell
    gh pr create --repo stolostron/gatekeeper-operator --title "Release ${RELEASE_VERSION}" --body ""
    ```

14. After the PR is merged (and any subsequent fixes), fetch the new commits:

    ```shell
    git fetch --prune upstream
    ```

15. Create and push a new release tag. This will trigger the GitHub actions release workflow to build and push the
    release image and create a new release on GitHub. Note that `upstream` is used as the remote name for this
    repository:

    ```shell
    RELEASE_VERSION="$(cat VERSION)"
    git tag -a ${RELEASE_VERSION} -m "${RELEASE_VERSION}" upstream/main
    git push upstream ${RELEASE_VERSION}
    ```

16. Disable [fast-forwarding](https://github.com/stolostron/gatekeeper-operator/actions/workflows/fast_forward.yaml) to
    prevent unwanted commits from going to the new release.
