package desiredstate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danieleborsaro/yago/pkg/desiredstate"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// =============================================================================
// BENCHMARKS: SERVICE LAYER OPERATIONS
// =============================================================================

func BenchmarkService_Initialize(b *testing.B) {
	baseDir := b.TempDir()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = desiredstate.NewService(baseDir, false)
	}
}

func BenchmarkService_Validate(b *testing.B) {
	testDir := b.TempDir()
	dsFile := filepath.Join(testDir, "service_bench.yaml")

	content := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: service-benchmark
  environment: dev
spec:
  components: []
`
	if err := os.WriteFile(dsFile, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	service := desiredstate.NewService(testDir, false)
	req := wrapper.ValidateRequest{
		DesiredStateFile: dsFile,
		Environment:      "dev",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.ValidateDesiredState(req)
	}
}

func BenchmarkService_Assemble(b *testing.B) {
	testDir := b.TempDir()
	dsFile := filepath.Join(testDir, "assemble_bench.yaml")
	cfgFile := filepath.Join(testDir, "config_bench.yaml")

	dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: assemble-benchmark
  environment: dev
spec:
  components: []
`
	cfgContent := `schema: "1.0.0"
namespace: yago
kind: Configuration
metadata:
  name: assemble-benchmark
  environment: dev
configuration:
  content:
    setting1: value1
`
	if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
		b.Fatalf("Failed to create desiredstate: %v", err)
	}
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0644); err != nil {
		b.Fatalf("Failed to create configuration: %v", err)
	}

	service := desiredstate.NewService(testDir, false)
	req := wrapper.AssembleRequest{
		DesiredStateFile: dsFile,
		ConfigFile:       cfgFile,
		Environment:      "dev",
		OutputFormat:     "yaml",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.AssembleDesiredState(req)
	}
}

// =============================================================================
// BENCHMARKS: LARGE FILE OPERATIONS
// =============================================================================

func BenchmarkService_ValidateLargeFile(b *testing.B) {
	testDir := b.TempDir()
	dsFile := filepath.Join(testDir, "large_file.yaml")

	// Generate a larger file with many components
	content := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: large-file-benchmark
  environment: dev
spec:
  components:`

	// Add 100 components
	for i := 0; i < 100; i++ {
		content += `
    - name: component` + string(rune(i+48)) + `
      type: service
      config:
        replicas: 3
        region: us-east-1`
	}

	if err := os.WriteFile(dsFile, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to create large file: %v", err)
	}

	service := desiredstate.NewService(testDir, false)
	req := wrapper.ValidateRequest{
		DesiredStateFile: dsFile,
		Environment:      "dev",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.ValidateDesiredState(req)
	}
}

func BenchmarkService_AssembleComplexConfiguration(b *testing.B) {
	testDir := b.TempDir()
	dsFile := filepath.Join(testDir, "complex_ds.yaml")
	cfgFile := filepath.Join(testDir, "complex_config.yaml")

	dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: complex-benchmark
  environment: dev
spec:
  components:
    - name: web-server
      type: service
    - name: database
      type: service
    - name: cache
      type: service
`

	// Generate complex configuration
	cfgContent := `schema: "1.0.0"
namespace: yago
kind: Configuration
metadata:
  name: complex-config-benchmark
  environment: dev
configuration:
  content:
    environments:
      dev:
        replicas: 1
        resources:
          cpu: "100m"
          memory: "128Mi"
      staging:
        replicas: 3
        resources:
          cpu: "500m"
          memory: "512Mi"
      prod:
        replicas: 10
        resources:
          cpu: "2000m"
          memory: "4Gi"
    wrappers:
      terraform:
        vars: "terraform.tfvars"
        backend: "s3"
      docker:
        image: "myapp:latest"
        registry: "ghcr.io"
      kubernetes:
        namespace: "default"
        context: "prod-cluster"
`
	if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
		b.Fatalf("Failed to create desiredstate: %v", err)
	}
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0644); err != nil {
		b.Fatalf("Failed to create complex config: %v", err)
	}

	service := desiredstate.NewService(testDir, false)
	req := wrapper.AssembleRequest{
		DesiredStateFile: dsFile,
		ConfigFile:       cfgFile,
		Environment:      "dev",
		OutputFormat:     "yaml",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.AssembleDesiredState(req)
	}
}

// =============================================================================
// BENCHMARKS: MEMORY ALLOCATION
// =============================================================================

func BenchmarkMemory_ServiceCreation(b *testing.B) {
	b.ReportAllocs()
	testDir := b.TempDir()

	for i := 0; i < b.N; i++ {
		_ = desiredstate.NewService(testDir, false)
	}
}

func BenchmarkMemory_Validate(b *testing.B) {
	b.ReportAllocs()

	testDir := b.TempDir()
	dsFile := filepath.Join(testDir, "mem_bench.yaml")
	content := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: memory-benchmark
  environment: dev
spec:
  components: []
`
	if err := os.WriteFile(dsFile, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	service := desiredstate.NewService(testDir, false)
	req := wrapper.ValidateRequest{
		DesiredStateFile: dsFile,
		Environment:      "dev",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.ValidateDesiredState(req)
	}
}

func BenchmarkMemory_Assemble(b *testing.B) {
	b.ReportAllocs()

	testDir := b.TempDir()
	dsFile := filepath.Join(testDir, "mem_ds.yaml")
	cfgFile := filepath.Join(testDir, "mem_cfg.yaml")

	dsContent := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: memory-assemble
  environment: dev
spec:
  components: []
`
	cfgContent := `schema: "1.0.0"
namespace: yago
kind: Configuration
metadata:
  name: memory-assemble
  environment: dev
configuration:
  content:
    key: value
`
	if err := os.WriteFile(dsFile, []byte(dsContent), 0644); err != nil {
		b.Fatalf("Failed to create desiredstate: %v", err)
	}
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0644); err != nil {
		b.Fatalf("Failed to create configuration: %v", err)
	}

	service := desiredstate.NewService(testDir, false)
	req := wrapper.AssembleRequest{
		DesiredStateFile: dsFile,
		ConfigFile:       cfgFile,
		Environment:      "dev",
		OutputFormat:     "yaml",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.AssembleDesiredState(req)
	}
}

//=============================================================================
// BENCHMARKS: COMPARISON BASELINES
// =============================================================================

func BenchmarkBaseline_SmallFileValidation(b *testing.B) {
	// Small file (<1KB) validation - baseline for comparison
	testDir := b.TempDir()
	dsFile := filepath.Join(testDir, "small.yaml")

	content := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: small
  environment: dev
spec:
  components: []
`
	if err := os.WriteFile(dsFile, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to create file: %v", err)
	}

	service := desiredstate.NewService(testDir, false)
	req := wrapper.ValidateRequest{
		DesiredStateFile: dsFile,
		Environment:      "dev",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.ValidateDesiredState(req)
	}
}

func BenchmarkBaseline_MediumFileValidation(b *testing.B) {
	// Medium file (~5KB) validation
	testDir := b.TempDir()
	dsFile := filepath.Join(testDir, "medium.yaml")

	content := `schema: "1.0.0"
namespace: yago
kind: DesiredState
metadata:
  name: medium
  environment: dev
spec:
  components:`

	for i := 0; i < 20; i++ {
		content += `
    - name: component` + string(rune(i+48)) + `
      type: service
      config:
        key1: value1
        key2: value2`
	}

	if err := os.WriteFile(dsFile, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to create file: %v", err)
	}

	service := desiredstate.NewService(testDir, false)
	req := wrapper.ValidateRequest{
		DesiredStateFile: dsFile,
		Environment:      "dev",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.ValidateDesiredState(req)
	}
}
