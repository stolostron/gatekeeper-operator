# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the Gatekeeper Operator repository, which installs Open Policy Agent (OPA) Gatekeeper using the Operator Lifecycle Manager (OLM). It references the `stolostron/gatekeeper` repository, a fork of upstream `open-policy-agent/gatekeeper`.

## Development Commands

Based on the documentation, the following Make commands are available:

```bash
# Format and lint code
make fmt lint

# Update Gatekeeper image and import manifests
make update-gatekeeper-image import-manifests update-bindata

# Create release artifacts
make release bundle

# Update generated manifests
make manifests
```

## Repository Structure

- `docs/` - Documentation including release processes and upgrade guides
- `.github/renovate.json` - Renovate bot configuration for dependency updates
- Release branches follow the pattern `release-X.Y`

## Architecture Notes

This is a Kubernetes operator built with operator-sdk that:
- Manages Gatekeeper installations via OLM
- Uses Go modules for dependency management
- Follows semantic versioning (stored in `VERSION` file)
- Supports optional `GATEKEEPER_VERSION` file for version overrides
- Uses Konflux for CI/CD and image building

## Release Process

The project uses a complex release process involving:
1. Creating release branches (`release-X.Y`)
2. Konflux application setup for CI/CD
3. Operator and bundle component updates
4. Version file management (`VERSION` and optional `GATEKEEPER_VERSION`)
5. Manifest updates and bundle generation

## Dependencies

- Go (with go.mod)
- operator-sdk (version specified in Makefile)
- OLM (Operator Lifecycle Manager)
- Konflux for builds and releases

## Notes

- This repository may have source code in different branches
- Check `release-*` branches for active development
- The main branch appears to contain minimal files
- Renovate is configured to handle containerfile and Go module updates