# Releasing Gatekeeper Operator

The following steps need to be done in order to update the Gatekeeper version and automate the
release of the Gatekeeper Operator using Konflux and the GitHub Actions release workflow.

- [Create a new release branch](#create-a-new-release-branch)
- [Create the Konflux application](#create-the-konflux-application)
- [Update the operator](#update-the-operator)
- [Tag the new release](#tag-the-new-release)

## Create a new release branch

1. Check the latest upstream release
   https://github.com/open-policy-agent/gatekeeper/releases/latest. Ensure an associated release
   branch has been created and updated in https://github.com/stolostron/gatekeeper and tagged with
   the correct version.
2. Make sure your clone is up-to-date and check out the latest `release-*` branch:

   ```shell
   STOLOSTRON="$(git remote -v | grep push | awk '/stolostron/ {print $1}')"
   git fetch --prune ${STOLOSTRON}
   latest_branch="$(git ls-remote ${STOLOSTRON} | grep -o release-[0-9]*\.[0-9]* | tail -1)"
   git checkout ${STOLOSTRON}/${latest_branch}
   ```

3. Set the desired operator version being released and push a new release branch based on the
   previous release:

   ```shell
   RELEASE_VERSION=1.2.3
   git push -u ${STOLOSTRON}/release-${RELEASE_VERSION%.*}
   ```

## Create the Konflux application

1. Create a new Konflux application for the new version in the
   [`releng/konflux-release-data`](https://gitlab.cee.redhat.com/releng/konflux-release-data/-/tree/main/tenants-config/cluster/stone-prd-rh01/tenants/gatekeeper-tenant)
   repo (VPN required), copying the most recent version into a new folder and doing a find/replace
   for the old version to the new version. Merge the MR there. This will trigger PRs to be created
   for each component in the repo.
2. Set the Konflux component to be updated (both must be handled):

   Operator:

   ```shell
   component=gatekeeper-operator
   ```

   Bundle:

   ```shell
   component=gatekeeper-operator-bundle
   ```

3. Checkout the Konflux PR branch (adjust the branch name if it's different), store the commit
   message, and reset the changes there:

   ```shell
   STOLOSTRON="$(git remote -v | grep push | awk '/stolostron/ {print $1}')"
   git fetch --prune ${STOLOSTRON}

   latest_branch="$(git ls-remote ${STOLOSTRON} | grep -o release-[0-9]*\.[0-9]* | tail -1)"
   xyver=${latest_branch#*-}
   git checkout ${STOLOSTRON}/appstudio-${component}-${xyver//./-}

   commit_msg=$(git show -s --format=%s)
   git reset ${STOLOSTRON}/${latest_branch}
   ```

4. Reflow the YAML with `yq`, delete the old Konflux configuration files, and stage the changes:

   ```shell
   for file in .tekton/gatekeeper-operator-*.yaml; do
        yq '.' -i ${file}
   done
   xyver=${RELEASE_VERSION%.*}
   find .tekton -name ${component}-[0-9]*.yaml -not -name *-${xyver//./-}-* -exec rm {} +
   git add .
   ```

5. With the changes staged, `git` should recognize the similarity between the files and indicate the
   old configuration files are being renamed and show the diff rather than showing it as different
   files. Handle/revert any changes that should stay, for example:

   - Add the `pathChanged()` expressions to the `pipelinesascode.tekton.dev/on-cel-expression`
     annotation
   - Add matching `spec.params`: `hermetic` and, for the operator (not bundle), `prefetch-input`

6. Update the image patch script with the new version (use `gsed` if on Mac):

   ```shell
   sed -E -i "s/[0-9]+-[0-9]+/${xyver//./-}/g" build/konflux-patch.sh
   ```

7. Commit the changes with the stored message and force push the commit:

   ```shell
   git commit -S -s -am "${commit_msg}"
   git push --force
   ```

## Update the operator

1. Once Konflux PRs have been merged for both components (operator and bundle), store the current
   and previous versions to release for use through the subsequent steps:

   ```shell
   RELEASE_VERSION=1.2.3
   RELEASE_PREV_VERSION=$(cat VERSION)
   ```

2. Pull the latest changes and checkout a new branch:

   ```shell
   STOLOSTRON="$(git remote -v | grep push | awk '/stolostron/ {print $1}')"
   git fetch --prune ${STOLOSTRON}
   git checkout ${STOLOSTRON}/release-${RELEASE_VERSION%.*}
   git checkout -b create-${RELEASE_VERSION}
   ```

3. Remove any `GATEKEEPER_VERSION` file if necessary:

   ```shell
   rm GATEKEEPER_VERSION
   ```

   If the latest upstream Gatekeeper version is a different z-version than the current operator
   version, instantiate a new Gatekeeper version file with the differing version:

   ```shell
   printf "1.2.3" > GATEKEEPER_VERSION
   ```

4. Update the `go.mod` with the matching Gatekeeper version:

   ```shell
   GATEKEEPER_VERSION=$(cat GATEKEEPER_VERSION 2>/dev/null || cat VERSION)
   go get github.com/open-policy-agent/gatekeeper/v3@v${GATEKEEPER_VERSION}
   go mod tidy
   ```

5. Update the version of the operator in the `VERSION` file, and update the base CSV `replaces`
   field:

   ```shell
   printf "${RELEASE_VERSION}" > VERSION
   printf "${RELEASE_PREV_VERSION}" > REPLACES_VERSION
   ```

6. Update the release manifest and update the bundle:

   ```shell
   make update-gatekeeper-image import-manifests update-bindata release bundle
   ```

   **NOTE**: If this is a z-stream release for a previous y-stream and there is a subsequent
   y-stream released, then `CHANNEL` and `DEFAULT_CHANNEL` must be set to remove the `stable`
   channel prior to running the release Make targets:

   ```shell
   export CHANNEL=$(cat VERSION | cut -d '.' -f 1-2)
   export DEFAULT_CHANNEL=${CHANNEL}
   ```

7. Check whether Gatekeeper permissions have changed or CRDs have been added. If they have, they'll
   need to also be updated in `controllers/gatekeeper_controller.go` so that the operator can grant
   those permissions and create those CRDs:

   ```shell
   git diff config/gatekeeper/rbac.authorization.k8s.io_v1_*
   git diff config/gatekeeper/apiextensions.k8s.io_v1_customresourcedefinition_*
   ```

   If permissions are updated, you need to run these commands to update the appropriate manifests:

   ```shell
   make manifests release bundle
   ```

8. Check for and fix any linting errors:

   ```shell
   make fmt lint
   ```

9. Commit above changes:

   ```shell
   git commit -S --signoff -am "Release ${RELEASE_VERSION}"
   ```

10. Push the changes in the branch to your fork (set `FORK` manually if you have more than one
    development fork):

    ```shell
    FORK="$(git remote -v | grep push | awk '!/stolostron/ {print $1}')"
    git push -u ${FORK} create-${RELEASE_VERSION}
    ```

11. Create a PR with the above changes and merge it. If using the `gh`
    [GitHub CLI](https://cli.github.com/) you can create the PR using:

    ```shell
    gh pr create --repo stolostron/gatekeeper-operator --title "Release v${RELEASE_VERSION}" --body ""  --base "release-${RELEASE_VERSION%.*}"
    ```

## Tag the new release

1. After the PR is merged (and any subsequent fixes), fetch the new commits (replace "release-X.Y"
   with the relevant release branch):

   ```shell
   branch=release-X.Y
   STOLOSTRON="$(git remote -v | grep push | awk '/stolostron/ {print $1}')"
   git fetch --prune ${STOLOSTRON}
   git checkout ${STOLOSTRON}/${branch}
   ```

2. Create and push a new release tag. This will trigger the GitHub actions release workflow to build
   and push the release image and create a new release on GitHub:

   ```shell
   RELEASE_VERSION="$(cat VERSION)"
   git tag -a ${RELEASE_VERSION} -m "${RELEASE_VERSION}"
   git push ${STOLOSTRON} ${RELEASE_VERSION}
   ```
