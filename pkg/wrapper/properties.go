package wrapper

import (
	"fmt"
	"maps"

	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// merges the named blocks of a configuration in the order given, each overriding the ones before, a deep merge also
// merges the maps inside them, a shallow one replaces them whole, which is what Terraform's merge() does
func MergeProperties(content map[string]interface{}, isDeep bool, names ...string) (map[string]interface{}, error) {
	engine := parser.NewStrategicMergeEngine(nil)
	merged := map[string]interface{}{}

	for _, name := range names {
		value, exists := content[name]
		if !exists || value == nil {
			logging.Debug("The configuration has no %s", name)
			continue
		}

		properties, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.NewConfigurationMalformedError(fmt.Sprintf("%s is not a map", name))
		}

		if !isDeep {
			maps.Copy(merged, properties)
			continue
		}
		var err error
		merged, err = engine.Merge(merged, properties)
		if err != nil {
			return nil, errors.Wrapf(errors.ErrParse, err, "failed to merge %s into project_properties", name)
		}
	}

	return merged, nil
}
