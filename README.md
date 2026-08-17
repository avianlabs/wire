# Wire: Automated Initialization in Go

[![Build Status](https://github.com/avianlabs/wire/actions/workflows/tests.yml/badge.svg?branch=main)](https://github.com/avianlabs/wire/actions)
[![godoc](https://pkg.go.dev/badge/github.com/avianlabs/wire.svg)][godoc]

> [!NOTE]
> This is a fork of the unmaintained original [Google Wire](https://github.com/google/wire)
> project. We have added more functionality over time, but aim to maintain this fork as a mostly
> drop in replacement for the original project.
>
> If you wish to migrate to this the import paths have changed from `github.com/google/wire` to `github.com/avianlabs/wire`.

Wire is a code generation tool that automates connecting components using
[dependency injection][]. Dependencies between components are represented in
Wire as function parameters, encouraging explicit initialization instead of
global variables. Because Wire operates without runtime state or reflection,
code written to be used with Wire is useful even for hand-written
initialization.

For an overview, see the [introductory blog post][].

[dependency injection]: https://en.wikipedia.org/wiki/Dependency_injection
[introductory blog post]: https://blog.golang.org/wire
[godoc]: https://pkg.go.dev/github.com/avianlabs/wire

## Installing

Install Wire by running:

```shell
go install github.com/avianlabs/wire/cmd/wire@latest
```

and ensuring that `$GOPATH/bin` is added to your `$PATH`.

## Changes from the original project

This fork aims to remain a drop-in replacement for `github.com/google/wire` —
existing provider sets and injectors work unchanged after updating the import
path. On top of that, it adds:

- **`wire.Singleton`** — wrap a provider function so generated injectors
  memoize its result. The memoizing wrapper is generated into the package
  that declares the singleton, so the provider runs at most once per binary
  and injectors generated into different packages share one instance.
  Errors are cached and cleanup functions are reference-counted across
  injectors. See the [User Guide][singleton guide] for details.
- **Multi-line generated calls** — provider calls in `wire_gen.go` are
  formatted with one argument per line, making generated code easier to read
  and producing smaller diffs when dependencies change.
- **Modern Go support** — dependencies are kept up to date so the tool works
  with current Go releases (Go 1.25 and later).
- **Upstream parity** — changes from the original repository are merged in
  periodically so the fork does not drift from upstream fixes.

[singleton guide]: ./docs/guide.md#singleton-providers

## Documentation

- [Tutorial][]
- [User Guide][]
- [Best Practices][]
- [FAQ][]

[Tutorial]: ./_tutorial/README.md
[Best Practices]: ./docs/best-practices.md
[FAQ]: ./docs/faq.md
[User Guide]: ./docs/guide.md

## Project status

The original Google Wire project is no longer maintained. This fork is
actively maintained by [Avian Labs](https://github.com/avianlabs): we merge
upstream fixes, keep the tool working with current Go releases, and add
functionality where it is broadly useful (see
[Changes from the original project](#changes-from-the-original-project)).
We aim to keep Wire simple and to remain a drop-in replacement for the
original, so new features are considered conservatively. Bug reports and
fixes are always welcome.

## Community

For questions, please use [GitHub Discussions](https://github.com/avianlabs/wire/discussions).

This project is covered by the Go [Code of Conduct][].

[Code of Conduct]: ./CODE_OF_CONDUCT.md
