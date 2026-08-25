# iacbom

> [!NOTE]
> **macOS users:** iacbom is not signed with an Apple Developer certificate
> and therefore not notarized. The Homebrew cask removes the quarantine flag
> automatically, but if you download a binary manually, macOS Gatekeeper may
> block it on first run. Unblock it with:
>
> ```bash
> xattr -dr com.apple.quarantine ./iacbom
> ```

**A Bill of Materials for Terraform and OpenTofu repositories.**

Traditional SBOMs describe software packages. iacbom describes the runtimes, providers, modules, automation, and supporting tools required to understand and reproduce an Infrastructure-as-Code repository.

```bash
iacbom
```

answers one question:

> What exactly makes up this IaC repository and its execution toolchain?

iacbom does **not** execute Terraform/OpenTofu and does not contact cloud providers or registries.

---

## What is iacbom?

`iacbom` scans a repository locally and produces a deterministic inventory of:

- which runtime (Terraform, OpenTofu, or both) the repo uses, and at which pinned version
- which providers are declared, with which constraints, and what is locked in `.terraform.lock.hcl`
- which modules are consumed (registry, git, local), including nested local module ancestry
- which state backend is configured
- which CI systems run IaC commands
- which tools make up the surrounding toolchain (linters, scanners, version managers, dependency automation)

It is read-only, local-first, fast, and useful without network access.

## What iacbom is not

iacbom does not replace:

- Checkov
- Trivy
- tfsec
- TFLint
- Terraform validate
- OpenTofu validate

Those tools answer different questions. iacbom does **not** scan cloud resources for security problems, detect secrets, evaluate whether dependencies are vulnerable, or execute Terraform/OpenTofu. It inventories what a repository is made of — nothing more.

## Why does an IaC BOM matter?

An IaC repository is more than its `.tf` files: reproducing an `apply` requires a specific binary, specific provider builds with specific checksums, specific module versions, and a set of CI and developer tools. Today this information lives scattered across a dozen files. iacbom collects it into one traceable, machine-readable document — every reported fact points back to the file (and line) it came from.

## Installation

```bash
# Homebrew (via tap)
brew install ojarosch/tap/iacbom

# Go toolchain
go install github.com/ojarosch/iacbom/cmd/iacbom@latest
```

Prebuilt binaries for Linux, macOS and Windows (amd64/arm64) are published on GitHub Releases with checksums.

## Quick start

Run it from the root of your infrastructure repository — the path is
optional and defaults to the current directory:

```bash
iacbom                        # scan current directory
iacbom ./path/to/repository   # or point it at one

iacbom --format json          # stable machine-readable output
iacbom --format cyclonedx-json
iacbom --format spdx-json     # SPDX 2.3 document
iacbom --verbose              # include evidence (file:line) for everything
iacbom --enrich               # query public registries for latest versions (network!)
iacbom providers              # subset views of the same BOM
iacbom modules
iacbom tools
iacbom diff old.json new.json # compare two saved BOMs (exit 1 when changed)
```

Exit codes:

| Code | Meaning                                        |
|------|------------------------------------------------|
| 0    | BOM generated / diff: no changes               |
| 1    | BOM generated with warnings / diff: changes found |
| 2    | fatal error                                    |

### Diffing

`iacbom diff` compares two previously generated JSON BOMs:

```text
iacbom diff infra-old.json -> infra-new.json

Providers
  ~ hashicorp/aws
      locked      6.8.0 -> 6.9.0
Modules
  ~ root/vpc
      version     6.0.1 -> 6.0.2
Tools
  + Checkov
```

Exit `1` when anything changed, `0` when identical — usable as a CI drift gate.

### Enrichment

By default iacbom is fully offline. With `--enrich` it queries the public Terraform/OpenTofu registries for the latest provider and registry-module versions and reports them next to your pinned versions:

```text
hashicorp/aws
    constraint: ~> 6.0
    locked:     6.8.0
    latest:     6.61.0
```

Lookup failures are warnings only; the BOM stays valid offline.

### Terragrunt and CDKTF support

- `terragrunt.hcl` files are scanned for `terraform { source = ... }` blocks (inventoried as modules) and `remote_state { backend = ... }` (inventoried as backends), with module paths namespaced as `terragrunt:<dir>`
- `cdktf.json` contributes `terraformProviders` / `terraformModules`, implies the Terraform runtime, and CDKTF is listed in the toolchain

## Example output

```text
iacbom

Runtime
  OpenTofu 1.11.2
    pinned:      1.11.2
    constraint:  >= 1.10, < 2.0

Providers
  hashicorp/aws
    constraint: ~> 6.0
    locked:     6.8.0

Modules
  terraform-aws-modules/vpc/aws 6.0.1
  ./modules/network (local module)

Backend
  s3

CI
  GitHub Actions
  .github/workflows/terraform.yml

Toolchain
  TFLint
  pre-commit
  tfenv

Summary
  1 runtime(s)
  1 provider(s)
  2 module(s)
  1 backend(s)
  3 tool(s)
```

## What gets detected?

### Terraform / OpenTofu support

Runtime detection uses multiple signals — never just the `terraform {}` HCL block name (OpenTofu uses identical syntax):

- `.terraform-version`, `.opentofu-version`
- `.tool-versions` (asdf)
- `mise.toml`, `mise.local.toml`
- `tofu` / `terraform` command usage in CI, Taskfiles, Makefiles, Justfiles
- `hashicorp/setup-terraform`, `opentofu/setup-opentofu` actions

A `required_version` value is reported as a *constraint*, never as a pin.

### Provider detection

- `required_providers` blocks in all root and nested modules, merged per canonical `namespace/name` source
- `.terraform.lock.hcl`: locked versions, constraints, and hashes preserved as-is
- Conflicting constraints across modules are represented side by side, not resolved:

```text
hashicorp/aws
  constraints:
    root:            ~> 6.0
    module.network:  >= 5.0
  locked:     6.8.0
```

### Module detection

Registry (`version` pins), git (`?ref=` extraction, never resolved over the network), local (with recursive discovery and loop protection), http, and other schemes. Nested local modules keep their ancestry (`module.platform.module.network`) in JSON.

### Backend detection

Any `backend "<type>"` block type is reported generically. No configuration values are exposed. Repositories without a backend block report `local/default`.

### CI detection

GitHub Actions workflows and GitLab CI files are inventoried; IaC-related third-party actions (e.g. `hashicorp/setup-terraform@v3`) are listed as toolchain dependencies.

### Toolchain detection

Data-driven catalog covering version managers (tfenv, tofuenv, asdf, mise), linters (TFLint), security scanners (Checkov, Trivy, tfsec, Terrascan, KICS), docs/cost tooling (terraform-docs, Infracost), pre-commit, Terragrunt, SOPS, Conftest, OPA, Renovate, and Dependabot. Detection comes from config file presence and command mentions — always with evidence.

### JSON output

```bash
iacbom --format json .
```

Stable schema (`schema_version: "1"`), sorted deterministically, no timestamps. Diagnostics from partial scans appear in a `diagnostics` array. CycloneDX 1.5 and SPDX 2.3 outputs map runtimes, providers, modules, and tools to components/packages with `iacbom` comments and properties.

## Security / privacy

- zero network requests unless `--enrich` is explicitly passed; enrichment only talks to public registries and never uploads repository content
- never reads `*.tfstate`, `*.tfvars` values, environment variables, or backend configuration values
- reports evidence locations only — never file content that could contain secrets

## Limitations

- versions that cannot be derived locally are reported as `unknown` rather than guessed
- git module refs are recorded but not resolved to commits
- mise/TOML parsing is line-based (covers standard `[tools]` declarations)
- YAML scanning is text-based by design; CI semantics are not executed
- conflicting provider constraints are shown, not solved
- SPDX output uses a fixed creation date to keep documents byte-reproducible

## Development

Build from source:

```bash
git clone https://github.com/ojarosch/iacbom
cd iacbom && go build -o iacbom ./cmd/iacbom
./iacbom testdata   # try it on a fixture
```

```bash
go test ./...
go vet ./...
go build ./cmd/iacbom
```

Fixtures live in `testdata/`; fuzz targets exist for HCL parsing, module URL classification, and command matching.

Releases are cut by pushing a `v*` tag; [.goreleaser.yaml](.goreleaser.yaml)
builds all platforms, publishes the GitHub Release, and updates the
[ojarosch/homebrew-tap](https://github.com/ojarosch/homebrew-tap) cask. The
workflow needs a `TAP_GITHUB_TOKEN` secret (a fine-grained PAT with push access
to the tap repository). Test releases locally with:

```bash
goreleaser release --snapshot --clean --skip=publish
```

## License

[Apache License 2.0](LICENSE)
