# Sous Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/)
with respect to its command line interface and HTTP interface.

## [0.4.1](//github.com/opentable/sous/compare/0.4.0...0.4.1)

### Fixed
- Status for updated deploys was being reported as if they were already stable.
  The stable vs. live statuses reported by the server each now have their own
  GDM snapshot so that this determination can be made properly.

## [0.4.0](//github.com/opentable/sous/compare/0.3.0...0.4.0)

### Added
- Conflicting GDM updates now retry, up to the number of deployments in their manifest.

### Changed
- Failed deploys to Singularity are now retried until they succeed or the GDM
  changes.

## [0.3.0](//github.com/opentable/sous/compare/0.2.1...0.3.0)

### Added
- Extra metadata tagged onto the Singularity deploys.
- `sous server` now treats its configured Docker registry as canonical, so
  that, e.g. regional mirrors can be used to reduce deploy latency.

### Changed

- Digested Docker image names no longer query the registry, which should reduce
  our requests count there.

## [0.2.1](//github.com/opentable/sous/compare/0.2.0...0.2.1)

### Added

- Adds Sous related-metadata to Singularity deploys for tracking and visibility purposes.

### Fixed

- In certain conditions, Sous would report a deploy as failed before it had completed.

## [0.2.0](//github.com/opentable/sous/compare/0.1.9...0.2.0) - 2017-03-06

### Added

- 'sous deploy' now returns a nonzero exit code when tasks for a deploy fail to start
  in Singularity. This makes it more suitable for unattended contexts like CD.

### Fixed

- Source locations with empty offsets and flavors no longer confuse 'sous plumbing status'.
  Previously 'sous plumbing status' (and 'sous deploy' which depends on it) were
  failing because they matched too many deployments when treating the empty
  offset as a wildcard. They now correctly treat it as a specific value.
- 'sous build' and 'sous deploy' previously appeared to hang when running long internal
  operations, they now inform the user there will be a wait.


## [0.1.9](//github.com/opentable/sous/compare/0.1.8...0.1.9) - 2017-02-16

### Added

- 'sous init -use-otpl-deploy' now supports flavors
  defined by otpl config directories in the `<cluster>.<flavor>` format.
  If there is a single flavor defined, behaviour is like before.
  Otherwise you may supply a -flavor flag to import configurations of a particular flavor.
- config.yaml now supports a new `User` field containing `Name` and `Email`.
  If set this info is sent to the server alongside all requests,
  and is used when committing state changes (as the --author flag).
- On first run (when there is no config file),
  and when a terminal is attached
  (meaning it's likely that a user is present),
  the user is prompted to provide their name and email address,
  and the URL of their local Sous server.
- `sous deploy`
  (and `sous plumbing status`)
  now await Singularity marking the indended deployment as active before returning.

### Fixed
- Deployment filters (which are used extensively) now treat "" dirs and flavors
  as real values, rather than wildcards.

## [0.1.8](//github.com/opentable/sous/compare/v0.1.7...0.1.8) - 2017-01-17

### Added
- 'sous init' -use-otpl-config now imports owners from singularity-request.json
- 'sous update' no longer requires -tag or -repo flags
  if they can be sniffed from the git repo context.
- Docs: Installation document added at doc/install.md

### Changed

- Logging, Server: Warn when artifacts are not resolvable.
- Logging: suppress full deployment diffs in debug (-d) mode,
  only print them in verbose -v mode.
- Sous Version outputs lines less than 80 characters long.

### Fixed

- Internal error caused by reading malformed YAML manifests resolved.
- SourceLocations now return more sensible parse errors when unmarshaling from JSON.
- Resolve errors now marshalled correctly by server.
- Server /status endpoint now returns latest status from AutoResolver rather than status at boot.

## [0.1.7](//github.com/opentable/sous/compare/v0.1.6...v0.1.7) 2017-01-19

### Added

- We are now able to easily release pre-built Mac binaries.
- Documentation about Sous' intended use for driving builds.
- Change in 'sous plumbing status' to support manifests that deploy to a subset of clusters.
- 'sous deploy' now waits by default until a deploy is complete.
  This makes it much more useful in unattended CI contexts.

### Changed

- Tweaks to Makefile and build process in general.

## [0.1.6](//github.com/opentable/sous/compare/v0.1.5...v0.1.6) 2017-01-19

Not documented.

## [0.1.5](//github.com/opentable/sous/compare/v0.1.4...v0.1.5) 2017-01-19

Not documented.

## [0.1.4](//github.com/opentable/sous/compare/v0.1.3...v0.1.4) 2017-01-19

Not documented.

## Sous 0.1

Sous 0.1 adds:
- A number of new features to the CLI.
  - `sous deploy` command (alpha)
  - `-flavor` flag, and support for flavors (see below)
  - `-source` flag which can be used instead of `-repo` and `-offset`
- Automatic migrations for the Docker image name cache.
- Consistent identifier parse and print round-tripping.
- Updates to various pieces of documentation.
- Nicer Singularity request names.

## Consistency

- Changes to the schema of the local Docker image name cache database no longer require user
  intervention and re-building the entire cache from source. Now, we track the schema, and
  migrate your cache as necessary.
- SourceIDs and SourceLocations now correctly round-trip from parse to string and back again.
- SourceLocations now have a single parse method, so we always get the same results and/or errors.
- We somehow abbreviated "Actual Deployment Set" "ADC" ?! That's fixed, we're now using "ADS".

### CLI

#### New command `sous deploy` (alpha)

Is intended to provide a hook for deploying single applications from a CI context.
Right now, this command works with a local GDM, modifying it, and running rectification
locally.
Over time, we will migrate how this command works whilst maintaining its interface and
semantics.
(The intention is that eventually 'sous deploy' will work by making an API call and
allowing the server to handle updating the GDM and rectifying.)

#### New flag `-flavor`

Actually, this is more than a flag, it affects the underlying data model, and the way
we think about how deployments are grouped.

Previously, Sous enabled at most a single deployment configuration per SourceLocation
per cluster. This model covers 90% of our use cases at OpenTable, but there are
exceptions.

We added "flavor" as a core concept in Sous, which allows multiple different deployment
configurations to be defined for a single codebase (SourceLocation) in each cluster. We
don't expect this feature to be used very much, but in those cases where configuration
needs to be more granular than per cluster, you now have that option available.

All commands that accept the `-source` flag (and/or the `-repo` and `-offset` flags) now
also accept the `-flavor` flag. Flavor is intended to be a short string made of
alphanumerics and possibly a hyphen, although we are not yet validating this string.
Each `-flavor` of a given `-source` is treated as a separate application, and has its
own manifest, allowing that application to be configured globally by a single manifest,
just like any other.

To create a new flavored application, you need to `sous init` with a `-flavor` flag. E.g.:

    sous init -flavor orange

From inside a repository would initiate a flavored manifest for that repo. Further calls
to `sous update`, `sous deploy`, etc, need to also specify the flavor `orange` to
work with that manifest. You can add an arbitrary number of flavors per SourceLocation,
and using a flavored manifest does not preclude you from also using a standard manifest
with no explicit flavor.

#### New flag `-source`

The `-source` flag is intended to be a replacement for the `-repo` and
`-offset` combination, for use in development environments. Note that we do not have
any plans to remove `-repo` and `-offset` since they may still be useful, especially
in scripting environments.

Source allows you to specify your SourceLocation in a single flag.
Source also performs additional validation,
ensuring that the source you pass can be handled by Sous end-to-end.
At present, that
means the repository must be a GitHub-based, in the form:

    github.com/<user>/<repo>

If your source code is not based in the root of the repository, you can add the offset
by separating it with a comma, e.g.:

    github.com/<user>/<repo>,<offset>

Because GitHub repository paths have a fixed format that Sous understands, you can
optionally use a slash instead of a comma, so the following is equivalent:

    github.com/<user>/<repo>/<offset>

(and offset can itself contain slashes if necessary, just like before).

### Documentation

We have made various documentation improvements, but there are definitely some that are
still out of date, which we will look to resolve in the coming weeks. Improvements made
in this release include:

- Better description of networking setup for Singularity deployments.
- Update to the deployment workflow documentation.
- Some fixes to the getting started document.

### Singularity request names

Up until now, Singularity request names looked something like this:

    github.comopentablereponameclustername

Which is not a great user experience, and has a large chance of causing naming collisions.
This version of Sous changes these names to use the form:

    <SourceLocation>:<Flavor>:<ClusterName>

E.g. for a simple repo with no offset or flavor, it looks like this:

    github.com>opentable>sous::cluster-name

With an offset and flavor, it expands to something like this:

    github.com>opentable>sous,offset:flavor-name:cluster-name


### Other

There have been numerous other small tweaks and fixes, mostly at code level to make our
own lives easier. We are also conscientiously working on improving test coverage, and this
cycle hit 54%, we expect to see that rise quickly now that we fail CI when it falls. You
can track test coverage at https://codecov.io/gh/opentable/sous.

For more gory detail, check out the [full list of commits between 0.0.1 and 0.1](https://github.com/opentable/sous/compare/v0.0.1...v0.1).