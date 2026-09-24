package secretsmanager

import (
	"fmt"

	"github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// The secrets live in the terraform wrapper's configuration, which the root modules read too.
const configurationWrapper = "terraform"

// Merged in this order, each overriding the ones before.
var projectPropertiesParts = []string{
	"project_properties_global",
	"project_properties_eco",
	"project_properties_proj",
	"project_properties_env",
}

type Service struct {
	*wrapper.BaseService
}

// NewService creates a new secretsmanager service instance.
func NewService(baseDir string, enableInterpolation bool) *Service {
	service := &Service{BaseService: wrapper.NewBaseService(baseDir, enableInterpolation)}
	service.SetAssembleHooks(service)
	return service
}

func (s *Service) AssembleSecrets(desiredStateFile, configFile, environment, cacheDir string) (*wrapper.AssembleResponse, error) {
	return s.Assemble(wrapper.AssembleRequest{
		DesiredStateFile: desiredStateFile,
		ConfigFile:       configFile,
		Environment:      environment,
		Wrapper:          configurationWrapper,
		CacheDirectory:   cacheDir,
		OutputFormat:     "json",
	})
}

func (s *Service) PreAssemble(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.AssembleContext) error {
	return nil
}

func (s *Service) PostDesiredStateAssemble(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.AssembleContext) error {
	return nil
}

func (s *Service) PostConfigurationAssemble(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.AssembleContext) error {
	if response.AssembledConfigurationContent == nil {
		return errors.NewConfigurationMalformedError("the assembled configuration has no content")
	}

	content, ok := wrapper.NormalizeMapForJSON(response.AssembledConfigurationContent).(map[string]interface{})
	if !ok {
		return errors.NewConfigurationMalformedError("the assembled configuration is not a map")
	}

	if _, exists := content["secrets"]; !exists {
		content["secrets"] = map[string]interface{}{}
	}

	projectProperties, err := mergeProjectProperties(content)
	if err != nil {
		return err
	}
	content["project_properties"] = projectProperties

	response.AssembledConfigurationContent = content
	return nil
}

func (s *Service) PostAssemble(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.AssembleContext) error {
	return nil
}

func (s *Service) PostCache(req wrapper.AssembleRequest, response *wrapper.AssembleResponse, ctx *wrapper.CacheContext) error {
	return nil
}

func mergeProjectProperties(content map[string]interface{}) (map[string]interface{}, error) {
	engine := parser.NewStrategicMergeEngine(nil)
	merged := map[string]interface{}{}

	for _, name := range projectPropertiesParts {
		value, exists := content[name]
		if !exists || value == nil {
			logging.Debug("The configuration has no %s", name)
			continue
		}

		properties, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.NewConfigurationMalformedError(fmt.Sprintf("%s is not a map", name))
		}

		var err error
		merged, err = engine.Merge(merged, properties)
		if err != nil {
			return nil, errors.Wrapf(errors.ErrParse, err, "failed to merge %s into project_properties", name)
		}
	}

	return merged, nil
}
