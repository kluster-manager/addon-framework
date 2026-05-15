# AGENTS.md

This file provides guidance to coding agents (e.g. Claude Code, claude.ai/code) when working with code in this repository.

## Repository purpose

Go module `open-cluster-management.io/addon-framework` — the upstream OCM (Open Cluster Management) framework for building **addons**: cluster-side components that the OCM hub installs/manages across a fleet of managed clusters. The framework wraps the OCM `ClusterManagementAddOn` / `ManagedClusterAddOn` CRDs with three deployment strategies (Go templates, Helm charts, AddOnTemplate), handles registration/RBAC, and exposes utilities for hosted vs. standard hosting modes. Library + example binaries.

This is a **library repo**: downstream addons import `open-cluster-management.io/addon-framework/pkg/...` to build their own controller. The fork is mirrored to `kluster-management/addon-framework` (the AppsCode mirror); the **upstream is `open-cluster-management-io/addon-framework`** and this repo tracks it.

## Architecture

- `pkg/addonfactory/` — high-level addon builders: `addonfactory.go` is the entry point, with `helm_agentaddon.go`, `template_agentaddon.go`, `addondeploymentconfig.go`, and shared `helper.go` for plumbing values into the chosen rendering engine.
- `pkg/addonmanager/` — the addon manager runtime:
  - `manager.go`, `base_manager.go`, `interface.go` — the manager surface.
  - `controllers/` — the reconcilers it runs.
  - `cloudevents/` — CloudEvents transport (used by `cloudevents` deployment style).
  - `constants/`, `addontesting/` — shared constants and test helpers.
- `pkg/agent/` — agent-side helpers used by the deployed cluster-local controllers.
- `pkg/assets/`, `pkg/index/`, `pkg/lease/`, `pkg/utils/`, `pkg/version/` — supporting packages.
- `pkg/cmd/` — CLI helpers.
- `pkg/dependencymagnet/` — Go-import-only package that forces vendoring of side-effect deps (e.g. `kustomize`).
- `examples/` — runnable example addons: `helloworld/`, `helloworld_agent/`, `helloworld_helm/`, `helloworld_hosted/`, plus `deploy/` and `rbac/` manifests. The Makefile's `deploy-helloworld*` / `undeploy-*` targets exercise them.
- `test/` — integration tests; `test/integration-test.mk` provides `envtest-setup` and `test-{kube,cloudevents,v1alpha1-kube}-integration`.
- `build/Dockerfile.example` — the example image.
- `Makefile` — thin wrapper that **vendors in OpenShift's `build-machinery-go/make/*.mk`**, so most "standard" targets (`build`, `test`, `verify`, `update`, `images`) come from `vendor/github.com/openshift/build-machinery-go/`. Cluster-side deploy targets are defined locally.
- `vendor/` — checked-in deps (required: the build machinery and `kustomize` machinery are pulled from there).

The repo's API surface is consumed via `pkg/`; do not break import paths.

## Common commands

The Makefile pulls in OpenShift's `build-machinery-go`, so several targets are inherited from there in addition to the locally-defined ones.

Standard development:

- `make build` (alias `make all`) — Go build (from `golang.mk`).
- `make test` — Go tests (from `golang.mk`).
- `make verify` — runs `verify-gocilint` (the local target on top of the machinery's `verify`).
- `make update` — code/manifest regen (from the machinery's `update` chain).
- `make images` — container image builds (from `images.mk`, using `EXAMPLE_IMAGE = addon-examples` and `build/Dockerfile.example`).
- `make ensure-kustomize` — install the pinned `KUSTOMIZE_VERSION = 4.5.5`.

Integration tests (require kubebuilder/envtest assets — `setup-envtest` is fetched automatically):

- `make test-kube-integration`
- `make test-cloudevents-integration`
- `make test-v1alpha1-kube`

End-to-end deploy/undeploy against a local OCM hub (target a running cluster via `KUBECONFIG`):

- `make deploy-ocm` (standard), `make deploy-hosted-ocm` (hosted), `make deploy-ocm-csr-token`, `make deploy-ocm-grpc-token`, `make deploy-ocm-cloudevents`.
- `make deploy-helloworld` / `…-helm` / `…-hosted` / `…-template` / `…-cloudevents`, `make deploy-busybox`, `make deploy-kubernetes-dashboard`, plus matching `undeploy-*` targets.
- `MANAGED_CLUSTER_NAME` (default `hub`) and the `HOSTED_MANAGED_*` env vars control the test cluster naming.

Run a single Go test:

```
go test ./pkg/addonfactory/... -run TestName -v
```

## Conventions

- Module path is `open-cluster-management.io/addon-framework` (upstream). Do not rename it; imports must use the upstream path.
- This is the **upstream** OCM addon-framework repo (mirrored on GitHub under `kluster-management/addon-framework`). Prefer rebasing onto upstream over diverging; isolate AppsCode-only patches so they replay cleanly.
- License: Apache-2.0 (`LICENSE`, `OWNERS`, `SECURITY.md`, `CONTRIBUTING.md` follow the OCM project conventions).
- Sign off commits (`git commit -s`) — DCO file is checked into the repo.
- Vendor directory is checked in **and load-bearing**: the Makefile `include`s `.mk` files from `vendor/github.com/openshift/build-machinery-go/make/`. Do not break the build machinery vendoring with a careless `go mod vendor`.
- Addon agents follow the `ClusterManagementAddOn` / `ManagedClusterAddOn` API contract from `open-cluster-management.io/api/addon/v1beta1` — when changing the framework's expected manifest shape, coordinate with the upstream OCM API repo.
- New deployment strategies: implement an `AgentAddon` builder under `pkg/addonfactory/`. Follow the existing `helm_agentaddon.go` / `template_agentaddon.go` patterns.
