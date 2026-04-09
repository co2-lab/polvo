# Security Policy

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report security issues by email to: **codeco@co2lab.io**

Include in your report:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

You will receive a response within 48 hours. If the issue is confirmed, we will release a patch as soon as possible and credit you in the release notes (unless you prefer to remain anonymous).

## Scope

The following are in scope:
- The Polvo binary (`app/`)
- The web IDE frontend (`ui/`)
- The HTTP API server
- The terminal WebSocket handler

Out of scope:
- Vulnerabilities in third-party dependencies (report those upstream)
- Issues requiring physical access to the machine
- Social engineering attacks

## Supported Versions

Only the latest release receives security fixes.
