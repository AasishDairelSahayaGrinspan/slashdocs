# Releasing slashdocs

A release is a git tag. Pushing a tag that matches `v<major>.<minor>.<patch>`
(optionally with a pre-release suffix such as `v1.2.0-rc.1`) runs
[`.github/workflows/release.yml`](../.github/workflows/release.yml), which:

1. Builds the server image for `linux/amd64` and `linux/arm64` and pushes it to
   Docker Hub as `razorpay/slashdocs:<version>` and `razorpay/slashdocs:<major>.<minor>`.
   Stable releases also move `razorpay/slashdocs:latest`; pre-releases do not.
2. Packages the Helm chart with `version` and `appVersion` set to the tag, so the
   chart defaults to the image that was just pushed.
3. Builds standalone CLI archives for Linux, macOS, and Windows (amd64 and
   arm64) with GoReleaser, and publishes a GitHub Release containing the archives, a
   `checksums.txt`, the chart `.tgz`, and a changelog grouped from the commit
   messages since the previous tag.

The GitHub Release is created only after the image push succeeds, so release
notes never point at an image that does not exist.

## Who can release

Tag creation, update, and deletion on `v*` are restricted by a repository
ruleset to repository administrators. Everyone else contributes through pull
requests; merging to `master` requires a passing CI run and a review from a
code owner ([`.github/CODEOWNERS`](../.github/CODEOWNERS)).

## Cutting a release

```bash
git checkout master
git pull --ff-only
git tag -a v0.1.0 -m "slashdocs v0.1.0"
git push origin v0.1.0
```

Then watch the **Release** workflow under Actions. When it finishes:

- `docker pull razorpay/slashdocs:0.1.0` works on both architectures.
- The GitHub Release lists the CLI archives, `checksums.txt`, and `slashdocs-0.1.0.tgz`.
- `go install github.com/razorpay/slashdocs/cli@v0.1.0` resolves.

Edit the generated release notes afterwards if a highlight deserves a sentence
the commit messages do not give it.

## Versioning

- Tags are [semver](https://semver.org) with a `v` prefix. The version string
  baked into the server (`/commit.txt`, `slashdocs-server --version`) and the
  CLI (`slashdocs --version`) is the tag itself, for example `v0.1.0`.
- The image tag and chart version drop the `v`: `razorpay/slashdocs:0.1.0`,
  `slashdocs-0.1.0.tgz`.
- `charts/slashdocs/Chart.yaml` keeps a placeholder `version`/`appVersion` in
  git; the release workflow overrides both from the tag at package time.
- Breaking changes to the API, CLI flags, environment variables, or chart
  values bump the minor version while we are pre-1.0, and the major version
  after that. Call them out under **Breaking changes** in the release notes.

## Docker Hub credentials

`release.yml` logs in to Docker Hub with the organization-level Actions secrets
`PUBLIC_DOCKER_USERNAME` and `PUBLIC_DOCKER_PASSWORD`, the same pair the other
public Razorpay repositories use to publish under the `razorpay` Docker Hub
organization. They are already visible to this repository; nothing needs to be
added per repo. If they are ever rotated or renamed at the organization level,
update the two `secrets.*` references in `release.yml`.

`GITHUB_TOKEN` is provided automatically and is used to create the GitHub Release.

## Re-running a release

Workflows are idempotent per tag: re-running the **Release** workflow re-pushes
the same image tags and replaces the GitHub Release contents (`mode: replace`).
If a tag was cut by mistake, delete the GitHub Release first, then the tag;
Docker Hub tags must be removed by hand in the Hub UI.
