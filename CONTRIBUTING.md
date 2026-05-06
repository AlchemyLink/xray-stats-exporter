# Contributing

Thank you for considering a contribution to xray-stats-exporter.

## License and Developer Certificate of Origin

This project is licensed under [AGPL-3.0-or-later](./LICENSE). By submitting a contribution, you agree that your contribution is licensed under the same terms.

### Sign-off requirement

We use the [Developer Certificate of Origin](https://developercertificate.org/) (DCO) v1.1 to certify the provenance of contributions. Every commit must include a `Signed-off-by:` trailer:

```text
Signed-off-by: Your Real Name <your.email@example.com>
```

Add it automatically with `git commit -s`. The name and email must match the commit author identity. CI rejects commits without a sign-off.

### Full DCO text

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project and the open source license(s) involved.
```

## Quality gates

Pull requests should pass:

```bash
go vet ./...
go test ./... -race -count=1
go mod tidy && git diff --exit-code go.mod go.sum
```

## Commit message conventions

Plain imperative summary. Example: `add inbound label to throughput metric`.

## Reporting security issues

If you discover a vulnerability, do not open a public GitHub issue. Use GitHub Security Advisories ("Report a vulnerability" button on the Security tab) or contact the maintainer directly.
