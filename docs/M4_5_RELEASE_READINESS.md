# M4.5 release readiness

M4.5 is the final qualification gate for the M4 development line and the intended v0.4.0 release candidate.

It adds no new product feature. Its job is to freeze and verify the release surface after M4.1-M4.4.

## Required state

- v0.3.0 remains an immutable ancestor of the candidate.
- `main` is ahead of v0.3.0 and not behind it.
- Go formatting, tests, and vet pass.
- Compose configuration validates.
- M4 deployment, maintenance, export, and observability documentation is present.
- permanent runtime workflows remain in the repository.
- temporary integration helpers are absent from the candidate tree.

## Release boundary

Passing M4.5 means the tree is ready to tag as v0.4.0. It does not itself create or move a tag, publish an OCI image, or create a GitHub Release.

The normal release action remains a separate explicit step so the qualified commit can be reviewed before publication.
