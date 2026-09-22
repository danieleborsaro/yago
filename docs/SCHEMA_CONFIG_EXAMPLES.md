# Schema Configuration Examples

This document demonstrates the different ways to specify the `schema-config.json` location in YAGO.

## Precedence Order

YAGO searches for `schema-config.json` in the following order (from lowest to highest priority):

1. **User home directory** (lowest): `~/.yago/schema-config.json` - Personal defaults
2. **Project root**: `./schema-config.json` - Project-specific configuration
3. **Environment variable**: `YAGO_SCHEMA_CONFIG=/path/to/config.json` - Context-specific override
4. **Command-line flag** (highest): `--schema-config /path/to/config.json` - Ultimate override

This development-friendly order allows you to:

- Set personal preferences once in your home directory
- Override with project-specific configs when working in a repository
- Use environment variables for CI/CD or multi-environment setups
- Use CLI flags for one-off testing or debugging

## Usage Examples

### 1. Using User Home Directory (Personal Defaults)

Create a personal default configuration that applies to all projects:

```bash
# Create the directory
mkdir -p ~/.yago

# Create your personal config
cat > ~/.yago/schema-config.json << 'EOF'
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": [
    "~/.yago/custom-schemas",
    "/opt/company/shared-schemas"
  ],
  "enablePlugins": true,
  "cacheSchemas": true
}
EOF

# All commands now use this config (unless overridden by project, env, or CLI)
yago ds validate -d desiredstate.yaml
```

**When to use**: Set up once and forget. Good for personal preferences like cache settings.

### 2. Using Project Root (Project-Specific)

Project-specific configuration that overrides your personal defaults:

```bash
# In your project root
cat > schema-config.json << 'EOF'
{
  "defaultSchemaPath": "schemas",
  "customSchemaPaths": ["./custom-schemas"],
  "enablePlugins": true,
  "cacheSchemas": true
}
EOF

# Commit to version control
git add schema-config.json
git commit -m "Add schema configuration"

# Anyone who clones the repo gets this config automatically
yago ds validate -d desiredstate.yaml
```

## Testing Configuration Resolution

Verify which configuration is being used:

```bash
# Enable verbose logging to see which config is loaded
yago ds validate -d desiredstate.yaml --verbose

# Look for log message like:
# "Loaded schema configuration from: /path/to/schema-config.json"
```

**When to use**: Team collaboration. Commit this to your repository for consistent team settings.

### 3. Using Environment Variable (Context Override)

Set a context-specific override for all commands in your session:

```bash
# Set the environment variable
export YAGO_SCHEMA_CONFIG=/opt/company/yago-schema-config.json

# Now all commands use this config (overrides home and project configs)
yago ds validate -d desiredstate.yaml
yago ds assemble -d desiredstate.yaml

# Add to your shell profile for persistence
echo 'export YAGO_SCHEMA_CONFIG=/opt/company/yago-schema-config.json' >> ~/.bashrc
```

**When to use**: CI/CD pipelines, multi-environment setups, or temporary team-wide overrides.

### 4. Using Command-Line Flag (Ultimate Override)

Override all other configurations with an explicit path for a single command:

```bash
# Use a specific config file for this command only
yago ds validate -d desiredstate.yaml --schema-config=/opt/custom/schema-config.json

# Works with any YAGO command
yago ds assemble -d desiredstate.yaml --schema-config=~/my-configs/schema-config.json
```

**When to use**: One-off testing, debugging, or validating against experimental schemas.

## Common Scenarios

### Scenario 1: New Developer Setup

A new developer joins your team:

```bash
# 1. Set up personal defaults once
mkdir -p ~/.yago
cat > ~/.yago/schema-config.json << 'EOF'
{
  "cacheSchemas": true,
  "enablePlugins": true
}
EOF

# 2. Clone the project (which has its own schema-config.json)
git clone https://github.com/company/project.git
cd project

# 3. Work normally - project config automatically overrides personal defaults
yago ds validate -d desiredstate.yaml
```

### Scenario 2: CI/CD Pipeline

Configure different schemas for different environments:

```bash
# In CI/CD pipeline
if [ "$ENVIRONMENT" = "production" ]; then
  export YAGO_SCHEMA_CONFIG=/etc/yago/production-schema-config.json
else
  export YAGO_SCHEMA_CONFIG=/etc/yago/development-schema-config.json
fi

yago ds validate -d desiredstate.yaml
```

**Why this works**: Environment variable overrides both personal and project configs.

### Scenario 3: Experimental Schema Testing

Test against a new schema version without modifying any configs:

```bash
# Keep all existing configs, but test with experimental schema
yago ds validate -d desiredstate.yaml --schema-config=/tmp/experimental-schema-config.json
```

**Why this works**: CLI flag has ultimate priority.

### Scenario 4: Multi-Project Developer

You work on multiple projects with different schema requirements:

```bash
# Personal defaults in ~/.yago/schema-config.json
# Project A has its own schema-config.json
# Project B has its own schema-config.json

cd ~/projects/project-a
yago ds validate -d desiredstate.yaml  # Uses project-a's config

cd ~/projects/project-b
yago ds validate -d desiredstate.yaml  # Uses project-b's config
```

**Why this works**: Project config (in current directory) overrides personal defaults.

## Home Directory Expansion

Tilde (`~`) is automatically expanded in all paths:

```bash
# These are equivalent
--schema-config=~/.yago/schema-config.json
--schema-config=/home/username/.yago/schema-config.json

# Also works in environment variables
export YAGO_SCHEMA_CONFIG=~/configs/schema-config.json
```

## Troubleshooting

### Check Which Config Is Being Used

```bash
# Run with verbose logging
yago ds validate -d desiredstate.yaml --verbose 2>&1 | grep "schema configuration"

# Example output:
# Loaded schema configuration from: /home/user/.yago/schema-config.json
```

### Verify Precedence

```bash
# Test each level
# 1. Default (project root)
yago ds validate -d desiredstate.yaml --verbose

# 2. User home directory (create it first)
mkdir -p ~/.yago && echo '{}' > ~/.yago/schema-config.json
yago ds validate -d desiredstate.yaml --verbose

# 3. Environment variable
export YAGO_SCHEMA_CONFIG=/tmp/test-config.json
echo '{}' > /tmp/test-config.json
yago ds validate -d desiredstate.yaml --verbose

# 4. CLI flag (highest priority)
yago ds validate -d desiredstate.yaml --schema-config=/tmp/cli-config.json --verbose
```

### Configuration Not Found

If YAGO warns "Failed to load schema config", check:

1. File exists: `ls -la ~/.yago/schema-config.json`
2. Valid JSON: `python -m json.tool ~/.yago/schema-config.json`
3. Readable: `cat ~/.yago/schema-config.json`
4. Correct path: Use absolute paths if relative paths fail
