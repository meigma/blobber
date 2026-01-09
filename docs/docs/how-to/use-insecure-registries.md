---
sidebar_position: 6
---

# How to Use Insecure Registries

Connect to registries without TLS for local development.

## Prerequisites

- blobber installed
- A registry running without TLS (e.g., local Docker registry)

## When to Use --insecure

Use this flag when connecting to:

- Local development registries (`localhost:5000`)
- Internal registries with self-signed certificates
- Test environments without TLS

**Never use `--insecure` for production registries.**

## Push to Insecure Registry

```bash
blobber push ./config localhost:5000/myimage:v1 --insecure
```

## Pull from Insecure Registry

```bash
blobber pull localhost:5000/myimage:v1 ./output --insecure
```

## List Files from Insecure Registry

```bash
blobber ls localhost:5000/myimage:v1 --insecure
```

## Stream Files from Insecure Registry

```bash
blobber cat localhost:5000/myimage:v1 app.yaml --insecure
```

## Running a Local Registry

Start a local registry for testing:

```bash
docker run -d -p 5000:5000 --name registry registry:2
```

Test with blobber:

```bash
mkdir test && echo "hello" > test/hello.txt
blobber push ./test localhost:5000/test:v1 --insecure
blobber ls localhost:5000/test:v1 --insecure
```

## Troubleshooting

### Error: certificate signed by unknown authority

The registry uses TLS but with an untrusted certificate. Options:

1. Add the certificate to your system's trust store
2. Use `--insecure` if this is a development environment

### Error: connection refused

Verify the registry is running:

```bash
curl http://localhost:5000/v2/
```

Should return `{}`.

## See Also

- [How to Authenticate](/docs/how-to/authenticate)
- [CLI Reference: push](/docs/reference/cli/push)
