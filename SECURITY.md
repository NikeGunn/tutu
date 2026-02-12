# Security Policy

## Supported Versions

| Version | Supported |
|---------|:---------:|
| 0.1.x (latest) | ✅ |
| < 0.1.0 | ❌ |

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

### How to Report

1. **Email:** Send details to **security@tutuengine.tech**
2. **GitHub Security Advisory:** Use GitHub's private vulnerability reporting at [https://github.com/Tutu-Engine/tutuengine/security/advisories/new](https://github.com/Tutu-Engine/tutuengine/security/advisories/new)

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Response Timeline

| Action | Timeline |
|--------|----------|
| Acknowledge receipt | Within 48 hours |
| Initial assessment | Within 5 business days |
| Patch development | Within 14 business days |
| Public disclosure | After patch is released |

### What to Expect

- A confirmation email acknowledging receipt
- Regular updates on progress
- Credit in the security advisory (unless you prefer anonymity)
- We will NOT take legal action against researchers who follow responsible disclosure

## Security Best Practices

When deploying TuTu Engine:

1. **Run behind a reverse proxy** (nginx, Caddy) in production
2. **Use TLS** for all API endpoints exposed to the internet
3. **Set `TUTU_HOME`** to a directory with appropriate permissions
4. **Do not expose port 11434** directly to the public internet without authentication
5. **Keep TuTu Engine updated** to the latest version
6. **Review the Dockerfile** — we use distroless images for minimal attack surface

## Scope

The following are in scope for security reports:

- TuTu Engine binary and API server
- MCP Gateway
- Credit system
- P2P networking protocol
- Authentication and authorization
- Data storage and handling
- Docker image security

The following are **out of scope**:

- Third-party AI models' content or behavior
- Social engineering attacks
- Denial of service attacks against development infrastructure
- Issues in dependencies (report upstream)

---

Thank you for helping keep TuTu Engine and our users safe.
