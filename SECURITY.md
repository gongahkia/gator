# Security Policy

## Supported Versions

Until the first tagged release, security fixes target `main` and the latest pre-release only.

| Version | Supported |
| --- | --- |
| `main` | Yes |
| `0.0.1-pre` | Yes |
| Earlier snapshots | No |

## Reporting a Vulnerability

Report suspected vulnerabilities privately through GitHub Security Advisories:

https://github.com/gongahkia/paw-cli/security/advisories/new

Do not open a public issue for a vulnerability before coordinated disclosure. No project security
email address or PGP key is currently configured.

Please include:

- affected `paw` version or commit
- operating system and install method
- clear reproduction steps
- expected and actual impact
- whether credentials, local files, or generated patches are involved

## Response Targets

These are targets, not guarantees:

- acknowledgement within 72 hours
- initial triage within 7 days
- fix and coordinated disclosure within 90 days

If a report is not in scope, maintainers will try to redirect it to the appropriate upstream
project or provider.

## Scope

In scope:

- the `paw` CLI binary and command behavior
- context gathering, compression, planning, edit, verify, resume, stats, and watch workflows
- patch application, path validation, and file write boundaries
- config parsing, environment handling, TLS options, and model transport request construction
- installer, release archive, checksum, SBOM, and CI/release workflow issues

Out of scope:

- vulnerabilities in upstream model providers, model APIs, Ollama, or external CLI tools
- model-generated code or advice unless caused by a `paw` implementation defect
- denial of service from intentionally huge repositories or hostile local files outside normal use
- social engineering, phishing, or attacks requiring prior compromise of the reporter's machine
- findings that require disabling documented security controls

## Safe Harbor

Good-faith research is authorized when it:

- avoids privacy violations, data destruction, persistence, and service disruption
- uses only the minimum access needed to demonstrate impact
- reports findings privately and allows time for coordinated disclosure
- does not access, modify, or exfiltrate third-party data

Maintainers will not pursue legal action for research that follows this policy.
