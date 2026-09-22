# Composer Handler Example

This example demonstrates how to create a custom component handler for Composer/Packagist packages in YAGO.

## Overview

The Composer handler manages PHP dependencies from Packagist.org, supporting:

- Package name resolution (vendor/package format)
- Version constraints (^1.0, ~2.3, *, etc.)
- Semantic versioning
- Exact version locking

## Files

- `composer_handler.go` - Handler implementation
- `composer_handler_test.go` - Test suite
- `README.md` - This file

## Installation

This is a demonstration example. To use it:

1. Copy `composer_handler.go` to your YAGO extension project
2. Import the package in your main.go:

   ```go
   import _ "your-project/composer"
   ```

3. The handler will auto-register via `init()`

## Usage

### YAML Configuration

```yaml
artifacts:
  symfony-framework:
    composer:
      package: symfony/symfony
      version: ^6.0
```

### Locking Versions

```bash
# Lock Composer packages to exact versions
yago lock --type composer

# This resolves version constraints (^6.0) to exact versions (6.2.1)
```

### Testing

```bash
go test -v ./composer/...
```

## Handler Structure

```go
type ComposerHandler struct {
    BaseComponentHandler
}

// Required methods:
// - Type() - Returns "composer"
// - ParseComponent() - Extracts package and version from YAML
// - ResolveVersion() - Resolves constraints to exact versions
// - CompareVersions() - Semantic version comparison
// - LockVersion() - Converts constraints to exact versions
// - UnlockVersion() - Resets to constraints
// - ValidateComponent() - Validates package name format
```

## Example Component

```go
comp := &VersionedComponent{
    Type:     ComponentTypeComposer,
    URL:      "https://packagist.org/packages/symfony/symfony",
    Version:  "^6.0",  // Constraint
    IsLocked: false,
    Metadata: map[string]interface{}{
        "package": "symfony/symfony",
    },
}

// Lock to exact version
handler.LockVersion(comp)
// comp.Version is now "6.2.1" (example)
// comp.IsLocked is now true
```

## Integration with Packagist API

For production use, integrate with the Packagist API:

```go
type PackagistClient struct {
    baseURL string
    httpClient *http.Client
}

func (c *PackagistClient) ResolveVersion(packageName, constraint string) (string, error) {
    // Query Packagist API for package versions
    // Filter by constraint
    // Return latest matching version
}
```

## Limitations

This example uses **placeholder** Packagist integration. For production:

1. Implement real Packagist API client
2. Add caching for API responses
3. Handle rate limiting
4. Support private Composer repositories
5. Add authentication for private packages

## References

- [Composer Documentation](https://getcomposer.org/doc/)
- [Packagist API](https://packagist.org/apidoc)
- [Semantic Versioning](https://semver.org/)
- [YAGO Handler Development Guide](../../../docs/COMPONENT_HANDLER_DEVELOPMENT.md)
