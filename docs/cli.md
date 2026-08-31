# JobDock CLI

The `jobdock` command operates the control plane without a browser session. Install the version-matched Linux amd64 or arm64 binary from the current stable release with:

```sh
curl -fsSL https://github.com/Alejandro-GZ/JobDock/releases/latest/download/install-cli.sh | sudo sh
```

The installer detects the supported platform, downloads the selected release archive, verifies it against the release `SHA256SUMS`, and only then installs `/usr/local/bin/jobdock`. Use `--version 1.2.3` to select an explicit release or `--bin-dir PATH` for another destination. No Go toolchain or repository checkout is required.

Run `jobdock --version` to display the product version and compatible server API. A CLI from a JobDock release targets server API `v1`; using the CLI from the same release as the server is recommended.

Create a personal access token in **Tokens**. The secret is shown once. For an interactive shell, export it without placing it on the command line:

```text
export JOBDOCK_URL=https://jobdock.example.com
export JOBDOCK_TOKEN='jdp_...'
```

For CI, use the platform's masked secret facility. A token can instead be read from a permission-restricted file using `JOBDOCK_TOKEN_FILE` or `--token-file`. JobDock never writes the token to output or error messages.

## Commands

```text
jobdock nodes
jobdock --version
jobdock jobs
jobdock run --name smoke --image alpine:3 --cpu 250 --memory 134217728 -- echo hello
jobdock logs -f <job-id>
jobdock logs --stream stderr <job-id>
jobdock stop <job-id>
jobdock download --output result.zip <job-id>
```

Add `--format json` before the command for stable JSON output. Human-readable tabular output is the default. `logs -f` uses server-acknowledged byte offsets, so polling and temporary disconnects do not download the complete log again.

Exit codes are stable for automation:

| Code | Meaning |
| ---: | --- |
| 0 | Success |
| 1 | Transport, server, or local I/O failure |
| 2 | Invalid command or arguments |
| 3 | Authentication, authorization, expired token, or insufficient scope |
| 4 | Resource not found |
| 5 | State conflict |

The available least-privilege scopes are `nodes:read`, `jobs:read`, `jobs:write`, `logs:read`, and `artifacts:read`. Token creation and revocation require a browser session and are recorded in the audit log. Revocation takes effect on the next request.
