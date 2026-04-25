# SIBA

**Structured Ink for Building Archives**

A module system for markdown documents. Headings are structure, HTML comments are annotations.

## Install

```bash
go install github.com/greyfolk99/siba/cmd/siba@latest
```

Or build from source:

```bash
git clone https://github.com/greyfolk99/siba.git
cd siba
go build -o siba ./cmd/siba
```

## Quick Start

```bash
# Initialize a project
siba init

# Read (streaming render)
siba cat docs/my-doc.md              # render to stdout
siba cat docs/my-doc.md#section      # specific section
siba cat --raw docs/my-doc.md        # raw source (no render)
siba head -n 5 docs/my-doc.md       # first 5 lines
siba tail -n 3 docs/my-doc.md       # last 3 lines

# Search (no render, index only)
siba ls                              # list all docs/templates
siba ls docs/my-doc.md               # list symbols in file
siba tree docs/my-doc.md             # heading tree
siba tree --deps                     # dependency tree
siba find "keyword"                  # search workspace

# Build
siba export                          # export to _export/{version}/
siba check docs/my-doc.md            # validate
siba check --json                    # JSON output (wrapped in {ok, data, errors})
```

## Concepts

### Headings = Structure

Heading levels define a tree. No open/close syntax needed.

```markdown
# API Spec
## Endpoints
### Authentication
### Routes
## Error Handling
```

### HTML Comments = Annotations

Directives go in HTML comments above headings. The document body stays pure markdown.

```markdown
<!-- @template architecture-doc -->
# Architecture Document

## Overview
(required by default - no annotation means required)

<!-- @default -->
## Deployment
Default deployment content. Inherited if not overridden.
```

### Variables

```markdown
<!-- @const service-name = "payment-api" -->
<!-- @const version: number = 2 -->
<!-- @let auth-type = "Bearer" -->

This is {{service-name}} v{{version}}.
All requests require {{auth-type}} token.
```

| Keyword | Reassign | Shadowing |
|---------|----------|-----------|
| `@const` | No | Forbidden |
| `@let` | Yes | Allowed in child scope |

### Templates and Contracts

A template defines required structure. Documents that extend it must fulfill the contract.

```markdown
<!-- @template api-spec -->
# API Spec
## Endpoints       (required)
## Error Handling  (required)

<!-- @default -->
## Changelog
Default changelog content.
```

```markdown
<!-- @doc payment-api -->
<!-- @extends api-spec -->
# Payment API
## Endpoints
POST /v1/payments ...
## Error Handling
400, 401, 500 ...
## Changelog
(inherits default if omitted)
```

### Control Flow

```markdown
<!-- @if env == "production" -->
## Production Config
...
<!-- @endif -->

<!-- @for endpoint in endpoints -->
### {{endpoint.name}}
{{endpoint.description}}
<!-- @endfor -->
```

### Access Control

```markdown
<!-- @const service-name = "identity" -->              public (default)
<!-- @const private db-password = "secret" -->          this document only
<!-- @const protected base-url = "/api/v1" -->          this + extending documents
```

### Reference Syntax

```markdown
{{variable}}                   Local variable
{{#section}}                   Section in same document
{{payment-api}}                Another document (by @doc name)
{{payment-api#overview}}       Section in another document
{{payment-api.version}}        Variable in another document
{{services/payment-api}}       Document by file path
\{{literal}}                   Escaped (outputs {{literal}})
```

### Packages

Go-module style. Git URLs as package names.

```toml
# module.toml
[module]
name = "github.com/hjseo/my-docs"
version = "1.0.0"

[dependencies]
"github.com/hjseo/architecture-templates" = "v1.2.0"

[scripts]
preexport = "echo starting"
postexport = "deploy.sh"
```

```bash
siba get github.com/hjseo/architecture-templates
siba tidy
siba run deploy
```

## Cycle Detection

SIBA detects circular references at runtime using a Nix-style evaluating marker pattern. Four levels:

1. **Variable** — `{{a}}` references `{{b}}` which references `{{a}}`
2. **Document** — doc-a inserts doc-b which inserts doc-a
3. **Extends** — template inheritance chain loops back
4. **Package** — package A depends on B which depends on A

## Project Structure

```
internal/
  ast/        AST definitions (Document, Heading, Variable, Reference, TypeExpr)
  parser/     Directive, heading, document, value parsers
  scope/      Scope chain (heading-based, variable resolution)
  types/      Type checker (TS subset: string, number, boolean, array, object, union)
  template/   Template contract validation + @default application
  refs/       Reference resolution + dependency graph + cycle detection
  control/    @if/@for evaluation
  validate/   Validation orchestration
  render/     Render pipeline (variable substitution, directive stripping, cycle detection)
  workspace/  Workspace management, module.toml parsing
  pkg/        Package manager
  scripts/    Build script runner
```

## Related Projects

- [siba-lsp](https://github.com/greyfolk99/siba-lsp) — LSP server
- [siba-viewer](https://github.com/greyfolk99/siba-viewer) — VSCode extension

## License

MIT
