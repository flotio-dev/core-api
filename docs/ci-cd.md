# CI and GitOps

The `CI` workflow runs Go formatting, vet, build, unit/race tests, coverage,
the existing Docker Compose/Postman integration suite, Swagger drift tests,
dependency review, CodeQL and Trivy. It builds one non-root production image,
scans that exact image, produces an SPDX SBOM, publishes it and optionally attests it, then
updates GitOps. It never invokes Kubernetes; Argo CD owns deployment and rollback.

Pull requests to `main` or `dev` run every validation without publishing or
receiving GitOps credentials. Pushes publish
`ghcr.io/flotio-dev/core-api:sha-<full-git-sha>` and hand only
`ghcr.io/flotio-dev/core-api@sha256:<digest>` to the matching dev or production
manifest. Pushes to `main` automatically compute the next SemVer version, create a GitHub release,
publish the SemVer alias, and update production GitOps after protected environment checks pass.

Required configuration:

- secrets `GH_APP_ID` and `GH_APP_PRIVATE_KEY`;
- optional `ATTESTATIONS_ENABLED=true` after private-repository attestations become
  available on the organization plan;
- GitHub App access only to `flotio-dev/k8s_config`, Contents write, with a
  narrowly scoped ruleset bypass for its main branch;
- protected `main`, `dev` and `production`, requiring `ci-success`;
- Secret Scanning, Push Protection and Dependabot security updates;
- the versioned CodeQL workflow, without duplicate default setup.

CI integration tests use only the test values committed in `.env.ci`; no
production application secret is exposed to the workflow. Security exceptions
belong in `.trivyignore.yaml` and require an exact ID, `Owner`, `Justification`
and finite `expired_at`.

Local checks:

```bash
test -z "$(gofmt -l .)"
go vet ./...
go test -race ./...
go build ./...
make swagger-check
docker compose -f docker-compose.ci.yml up -d --build
docker build --pull -t core-api:local .
actionlint .github/workflows/ci.yaml
```

Release procedure: merge to `main` and let the `auto-release` workflow tag the release, generate notes, and publish the SemVer image alias.
