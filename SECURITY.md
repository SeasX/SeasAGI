# Security Policy

## Reporting a Vulnerability

We take the security of SeasAGI seriously. If you believe you have found a security vulnerability, please report it to us as described below.

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to [security@seasx.ai](mailto:security@seasx.ai).

You should receive a response within 48 hours. If for some reason you do not, please follow up via email to ensure we received your original message.

## What to Include

To help us better understand the nature and scope of the issue, please include:

- Type of issue (e.g. buffer overflow, SQL injection, cross-site scripting, etc.)
- Full paths of source file(s) related to the manifestation of the issue
- The location of the affected source code (tag/branch/commit or direct URL)
- Any special configuration required to reproduce the issue
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit it

## Preferred Languages

We prefer all communications to be in English.

## Policy

- We will acknowledge receipt of your vulnerability report within 48 hours
- We will send you a more detailed response within 72 hours indicating next steps
- We will keep you informed of the progress towards a fix and full announcement
- We will notify you when the reported vulnerability is remediated

## Scope

This security policy applies to all projects within the SeasAGI organization:

- SeasAGI (repository root)
- SeasAGI-Client
- SeasAGI-Server

## Secure Configuration

### Required Environment Variables

The following environment variables **MUST** be set in production deployments:

| Variable | Description |
|----------|-------------|
| `JWT_SECRET` | Secret key for signing JWT access tokens (minimum 32 characters) |
| `JWT_REFRESH_SECRET` | Secret key for signing JWT refresh tokens (minimum 32 characters) |
| `ADMIN_SECRET` | Secret key for admin API access |

### Recommended Security Headers

When deploying behind nginx, ensure the following security headers are set:

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Strict-Transport-Security: max-age=31536000; includeSubDomains`
- `Referrer-Policy: strict-origin-when-cross-origin`