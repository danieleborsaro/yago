package schema

// SchemaVersion represents a schema version
type SchemaVersion string

// SchemaType represents different types of schemas
type SchemaType string

const (
	SchemaTypeDesiredStateMeta      SchemaType = "desiredstate_meta"
	SchemaTypeDesiredStateAssembled SchemaType = "desiredstate_assembled"
	SchemaTypeConfigurationMeta     SchemaType = "configuration_meta"
	SchemaTypeConfigurationContent  SchemaType = "configuration_content"
)
