# YAGO (Yet Another GitOps) Business Logic Structure

## Overview

The YAGO project follows a layered architecture pattern with clear separation of concerns. The business logic is organized into distinct layers: CLI, Wrappers, Core Processing, Schema Management, and Utilities.

## Architecture Layers

```
┌─────────────────────────────────────────────────────────────────┐
│                        CLI Layer                                │
├─────────────────────────────────────────────────────────────────┤
│                     Wrapper Layer                              │
├─────────────────────────────────────────────────────────────────┤
│                   Core Processing Layer                        │
├─────────────────────────────────────────────────────────────────┤
│                     Schema Layer                               │
├─────────────────────────────────────────────────────────────────┤
│                     Utility Layer                              │
└─────────────────────────────────────────────────────────────────┘
```

## Class Hierarchy and Components

### 1. CLI Layer (`internal/cli/`)

#### `RootCommand`

- **Purpose**: Entry point for the CLI application
- **Responsibilities**:
  - Command-line argument parsing
  - Global flag management (`--verbose`, `--log-level`)
  - Environment variable processing (`LOG_LEVEL`)
  - Logging configuration initialization
- **Dependencies**: Cobra CLI framework
- **Key Methods**:
  - `Execute()`: Main CLI execution
  - `PersistentPreRun()`: Global setup (logging, env vars)

### 2. Wrapper Layer (`internal/wrappers/`)

#### `DesiredStateWrapper` (`internal/wrappers/desiredstate/`)

##### `DesiredStateConfig`

- **Purpose**: Configuration container for desiredstate operations
- **Properties**:
  - `BaseDir`: Working directory
  - `Environment`: Target environment
  - `DesiredStateFile`: Source file path
  - `DestinationFile`: Target file path
  - `IsDryRun`: Simulation mode flag
  - `IsInterpolation`: Lookup processing flag

##### Command Handlers

- **`validateCommand`**: GitOps desired state validation
- **`promoteCommand`**: Environment promotion with AWS integration
- **`printschemaCommand`**: Schema definition display

**Command Pattern Implementation**:

```go
type Command interface {
    Execute(config *DesiredStateConfig, args []string) error
}
```

### 3. Core Processing Layer (`internal/parser/`)

#### `YAMLHandler`

- **Purpose**: Central YAML processing and manipulation engine
- **Responsibilities**:
  - YAML file loading and parsing
  - Lookup function processing and interpolation
  - Schema validation coordination
  - Content transformation and assembly

##### Key Interfaces

```go
type LookupFunction func(
    content map[string]interface{}, 
    baseDir string, 
    isInterpolation bool, 
    helpers map[string]interface{}
) error
```

##### Core Methods

- `LoadFile()`: Single file processing
- `LoadFiles()`: Multiple file assembly
- `ValidateSchemaAutoDetect()`: Automatic schema validation
- `RegisterLookupFunction()`: Custom lookup registration

#### Lookup Function System

Implements the **Strategy Pattern** for dynamic content resolution:

##### `getEnvValueLookup`

- **Pattern**: `[[gitops.getEnvValue(ENV_VAR_NAME)]]`
- **Purpose**: Environment variable interpolation

##### `getYamlValueLookup`

- **Pattern**: `[[gitops.getYamlValue(path.to.value)]]`
- **Purpose**: Cross-file YAML value lookup

##### `getFileContentLookup`

- **Pattern**: `[[gitops.getFileContent(file.txt)]]`
- **Purpose**: Raw file content inclusion

##### `getConfigYamlValueLookup`

- **Pattern**: `[[gitops.getConfigYamlValue(config.path)]]`
- **Purpose**: Configuration-specific value lookup

### 4. Schema Management Layer (`internal/schema/`)

#### `SchemaManager`

- **Purpose**: Centralized schema validation and version management
- **Pattern**: **Factory Pattern** for schema version creation

##### `SchemaVersion` Enumeration

```go
const (
    Version1_0_0  SchemaVersion = "1.0.0"
    Version2_0_0  SchemaVersion = "2.0.0"
    // ... through 4.2.0
    VersionLatest SchemaVersion = "latest"
)
```

##### `SchemaType` Classification

```go
const (
    SchemaTypeDesiredStateMeta      SchemaType = "desiredstate_meta"
    SchemaTypeDesiredStateAssembled SchemaType = "desiredstate_assembled"
    SchemaTypeConfigurationMeta     SchemaType = "configuration_meta"
    SchemaTypeConfigurationContent  SchemaType = "configuration_content"
)
```

#### `BaseSchema` Interface

- **Purpose**: Defines contract for schema implementations
- **Methods**:
  - `LoadSchema()`: Schema definition loading
  - `Validate()`: JSON Schema validation
  - `GetSchema()`: Schema retrieval by type

#### Version Detection Logic

Implements **dual format support**:

- **Legacy Format**: `schema: 3.8.0`
- **Modern Format**: `schema: 3.8.0`

##### Normalization Process

```go
func normalizeContentForValidation(content, version) {
    // Ensures 'schemaVersion' field exists for JSON schema validation
    // while preserving original 'schema' field format
}
```

### 5. Utility Layer (`internal/utils/`)

#### Logging Subsystem (`internal/utils/logging/`)

##### `Logger`

- **Purpose**: Structured logging with color support
- **Features**:
  - Multiple log levels (DEBUG, INFO, WARN, ERROR, FATAL)
  - Colored console output
  - Caller information display
  - Environment variable integration

##### Log Level Hierarchy

```go
type LogLevel int
const (
    DEBUG LogLevel = iota
    INFO
    WARN
    ERROR
    FATAL
)
```

#### Command Execution (`internal/utils/command/`)

##### `ShellExecutor`

- **Purpose**: External command execution with advanced features
- **Capabilities**:
  - Working directory management
  - Environment variable injection
  - Timeout handling
  - Output buffering and streaming
  - Exit code management

##### `Config` Structure

```go
type Config struct {
    WorkingDir               string
    Env                      map[string]string
    Timeout                  time.Duration
    IsBufferedOutput         bool
    IsShowOutput             bool
    IsMeasureDuration        bool
    IsRealtimeOutput         bool
    IsRedirectStderrToStdout bool
}
```

#### Repository Management (`internal/utils/repo/`)

##### `Repository`

- **Purpose**: Git repository operations
- **Features**:
  - Repository cloning and management
  - Branch and tag operations
  - Authentication handling
  - Change detection

##### `RepoConfig`

```go
type RepoConfig struct {
    Username string
    Token    string
    Logger   *logging.Logger
}
```

#### Error Handling (`internal/utils/errors/`)

##### Status Code System

```go
const (
    StatusOK                    Status = 0
    StatusParam                 Status = 1
    StatusTerraformError        Status = 2
    StatusMissingTool           Status = 3
    StatusDesiredStateMissing   Status = 4
    StatusDesiredStateMalformed Status = 5
    StatusConfigMissing         Status = 6
    StatusConfigMalformed       Status = 7
    StatusUndefined             Status = 255
)
```

## Design Patterns Used

### 1. **Strategy Pattern**

- **Implementation**: Lookup function system
- **Purpose**: Pluggable content interpolation strategies
- **Benefits**: Extensible lookup mechanism

### 2. **Factory Pattern**

- **Implementation**: Schema version management
- **Purpose**: Version-specific schema creation
- **Benefits**: Centralized version control

### 3. **Command Pattern**

- **Implementation**: CLI command structure
- **Purpose**: Encapsulated command execution
- **Benefits**: Testable, modular command logic

### 4. **Decorator Pattern**

- **Implementation**: YAML content processing pipeline
- **Purpose**: Layered content transformation
- **Benefits**: Composable processing stages

### 5. **Builder Pattern**

- **Implementation**: Configuration object construction
- **Purpose**: Complex configuration assembly
- **Benefits**: Flexible configuration creation

## Data Flow Architecture

```
CLI Input → Command Parser → Wrapper Logic → YAML Handler → Schema Validation
    ↓           ↓               ↓              ↓               ↓
Validation  Configuration  File Loading   Interpolation   JSON Schema
    ↓           ↓               ↓              ↓               ↓
Processing  Environment    Content Merge  Lookup Exec.    Result Output
```

## Key Business Rules

### Schema Version Compatibility

- Supports both `schemaVersion` (legacy) and `schema` (modern) field formats
- Automatic version detection and normalization
- Backward compatibility across all schema versions 1.0.0 through 4.2.0

### Interpolation Processing

- Ordered lookup execution for deterministic results
- Environment variable precedence over file-based lookups
- Nested lookup support with circular reference detection

### Error Handling Strategy

- Fail-fast validation for early error detection
- Detailed error context with file paths and line numbers
- Graceful degradation for non-critical operations

### Security Considerations

- Environment variable sandboxing
- File path validation and sanitization
- Authentication token management for repository operations

## Extension Points

The architecture provides several extension points for future enhancement:

1. **Custom Lookup Functions**: Plugin-style lookup registration
2. **Additional Schema Types**: New schema type registration
3. **Command Extensions**: New wrapper command addition
4. **Output Formatters**: Pluggable output format support
5. **Authentication Providers**: Multiple auth mechanism support

This structure provides a maintainable, testable, and extensible foundation for GitOps operations while maintaining clean separation of concerns and clear data flow.
