package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const moduleImportPrefix = "github.com/srapi/srapi/apps/api/internal/modules/"
const requiredGoVersion = "1.26.3"
const maxRuntimeHTTPCompatibilityLines = 120
const maxRuntimeFileLines = 2200

var allowedContractImports = map[string]map[string]bool{
	"auth": {
		"users": true,
	},
	"models": {
		"capabilities": true,
	},
	"operations": {
		"usage": true,
	},
	"provider_adapters": {
		"accounts":  true,
		"models":    true,
		"providers": true,
	},
	"scheduler": {
		"accounts":     true,
		"capabilities": true,
		"models":       true,
		"providers":    true,
	},
}

func TestModuleProductionCodeOnlyImportsOtherModuleContracts(t *testing.T) {
	root := filepath.Clean("../modules")
	var violations []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		currentModule := moduleName(root, path)
		if currentModule == "" {
			return nil
		}
		imports, err := fileImports(path)
		if err != nil {
			return err
		}
		for _, imported := range imports {
			if strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/ent") {
				violations = append(violations, path+": module production code must not import Ent packages")
				continue
			}
			if imported == "github.com/srapi/srapi/apps/api/internal/httpserver" || strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/httpserver/") {
				violations = append(violations, path+": module production code must not import HTTP server packages")
				continue
			}
			if !strings.HasPrefix(imported, moduleImportPrefix) {
				continue
			}
			rest := strings.TrimPrefix(imported, moduleImportPrefix)
			parts := strings.Split(rest, "/")
			if len(parts) < 2 {
				continue
			}
			importedModule := parts[0]
			importedLayer := parts[1]
			if importedModule == currentModule {
				continue
			}
			if importedLayer != "contract" {
				violations = append(violations, path+": cross-module import must target contract, got "+imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("module boundary violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestModuleContractsOnlyUseStableDependencies(t *testing.T) {
	root := filepath.Clean("../modules")
	var violations []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		module := moduleName(root, path)
		if module == "" || moduleLayer(root, path) != "contract" {
			return nil
		}
		imports, err := fileImports(path)
		if err != nil {
			return err
		}
		for _, imported := range imports {
			if strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/ent") {
				violations = append(violations, path+": contract must not import Ent packages")
				continue
			}
			if imported == "github.com/srapi/srapi/apps/api/internal/openapi" || strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/openapi/") {
				violations = append(violations, path+": contract must not import generated OpenAPI DTOs")
				continue
			}
			if imported == "github.com/srapi/srapi/apps/api/internal/httpserver" || strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/httpserver/") {
				violations = append(violations, path+": contract must not import HTTP server packages")
				continue
			}
			if !strings.HasPrefix(imported, moduleImportPrefix) {
				continue
			}
			importedModule, importedLayer := importedModuleLayer(imported)
			if importedModule == "" || importedModule == module {
				continue
			}
			if importedLayer != "contract" {
				violations = append(violations, path+": contract cross-module import must target contract, got "+imported)
				continue
			}
			if !allowedContractImports[module][importedModule] {
				violations = append(violations, path+": contract import must be listed in allowedContractImports, got "+imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("module contract dependency violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestCommandEntryPointOnlyImportsBootstrapPackages(t *testing.T) {
	path := filepath.Clean("../../cmd/srapi/main.go")
	imports, err := fileImports(path)
	if err != nil {
		t.Fatal(err)
	}

	allowed := map[string]bool{
		"context":   true,
		"flag":      true,
		"os":        true,
		"os/signal": true,
		"syscall":   true,
		"time":      true,

		"github.com/srapi/srapi/apps/api/internal/app":             true,
		"github.com/srapi/srapi/apps/api/internal/config":          true,
		"github.com/srapi/srapi/apps/api/internal/platform/logger": true,
	}

	var violations []string
	for _, imported := range imports {
		if strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/ent") {
			violations = append(violations, path+": cmd entrypoint must not import Ent packages")
			continue
		}
		if strings.HasPrefix(imported, moduleImportPrefix) {
			violations = append(violations, path+": cmd entrypoint must not import business modules, got "+imported)
			continue
		}
		if strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/") && !allowed[imported] {
			violations = append(violations, path+": cmd entrypoint must not import internal packages outside bootstrap scope, got "+imported)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("command entrypoint boundary violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestAppBootstrapOnlyImportsBootstrapPackages(t *testing.T) {
	path := filepath.Clean("../../internal/app/app.go")
	imports, err := fileImports(path)
	if err != nil {
		t.Fatal(err)
	}

	allowed := map[string]bool{
		"context":  true,
		"errors":   true,
		"fmt":      true,
		"log/slog": true,
		"net/http": true,
		"time":     true,

		"github.com/srapi/srapi/apps/api/internal/config":                           true,
		"github.com/srapi/srapi/apps/api/internal/httpserver":                       true,
		"github.com/srapi/srapi/apps/api/internal/persistence/entstore":             true,
		"github.com/srapi/srapi/apps/api/internal/persistence/entstore/scheduler":   true,
		"github.com/srapi/srapi/apps/api/internal/persistence/redisstore/realtime":  true,
		"github.com/srapi/srapi/apps/api/internal/persistence/redisstore/scheduler": true,
		"github.com/srapi/srapi/apps/api/internal/platform/db":                      true,
		"github.com/srapi/srapi/apps/api/internal/platform/otel":                    true,
		"github.com/srapi/srapi/apps/api/internal/platform/redis":                   true,
		"github.com/srapi/srapi/apps/api/internal/workers/health_probe":             true,
		"github.com/srapi/srapi/apps/api/internal/workers/order_expirer":            true,
		"github.com/srapi/srapi/apps/api/internal/workers/outbox":                   true,
		"github.com/srapi/srapi/apps/api/internal/workers/quality_eval":             true,
		"github.com/srapi/srapi/apps/api/internal/workers/retention":                true,
		"github.com/srapi/srapi/apps/api/internal/workers/balance_charger":          true,
		"github.com/srapi/srapi/apps/api/internal/workers/subscription_expirer":     true,
	}

	var violations []string
	for _, imported := range imports {
		if strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/ent") {
			violations = append(violations, path+": app bootstrap must not import Ent packages")
			continue
		}
		if strings.HasPrefix(imported, moduleImportPrefix) {
			violations = append(violations, path+": app bootstrap must not import business modules, got "+imported)
			continue
		}
		if strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/") && !allowed[imported] {
			violations = append(violations, path+": app bootstrap must not import internal packages outside bootstrap scope, got "+imported)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("app bootstrap dependency violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestHTTPServerDoesNotImportEnt(t *testing.T) {
	root := filepath.Clean("../httpserver")
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		imports, err := fileImports(path)
		if err != nil {
			return err
		}
		for _, imported := range imports {
			if strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/ent") {
				violations = append(violations, path+": httpserver must not import Ent packages")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("httpserver Ent dependency violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestHTTPRuntimeFilesStayPartitioned(t *testing.T) {
	root := filepath.Clean("../httpserver")
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		base := filepath.Base(path)
		lines, err := fileLineCount(path)
		if err != nil {
			return err
		}
		switch {
		case base == "runtime_http.go" && lines > maxRuntimeHTTPCompatibilityLines:
			violations = append(violations, path+": runtime_http.go must remain a thin compatibility shell; split route-family code into runtime_*.go files")
		case strings.HasPrefix(base, "runtime_") && lines > maxRuntimeFileLines:
			violations = append(violations, path+": runtime route-family file is too large; split by ownership before adding more handlers")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("HTTP runtime partition violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestWorkersOnlyDependOnContractsAndServices(t *testing.T) {
	root := filepath.Clean("../workers")
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		imports, err := fileImports(path)
		if err != nil {
			return err
		}
		for _, imported := range imports {
			switch {
			case strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/ent"):
				violations = append(violations, path+": worker must not import Ent packages")
			case imported == "github.com/srapi/srapi/apps/api/internal/httpserver" || strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/httpserver/"):
				violations = append(violations, path+": worker must not import HTTP server packages")
			case strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/persistence/"):
				violations = append(violations, path+": worker must not import persistence packages directly, got "+imported)
			case strings.HasPrefix(imported, moduleImportPrefix):
				_, importedLayer := importedModuleLayer(imported)
				if importedLayer != "contract" && importedLayer != "service" {
					violations = append(violations, path+": worker may only import module contracts/services, got "+imported)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("worker dependency violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestEntStoresOnlyDependOnContractsAndEnt(t *testing.T) {
	root := filepath.Clean("../persistence/entstore")
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		imports, err := fileImports(path)
		if err != nil {
			return err
		}
		for _, imported := range imports {
			switch {
			case strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/ent"):
				continue
			case strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/persistence/entstore"):
				continue
			case strings.HasPrefix(imported, moduleImportPrefix):
				importedModule, importedLayer := importedModuleLayer(imported)
				if importedModule == "" || importedLayer != "contract" {
					violations = append(violations, path+": Ent store may only import module contracts, got "+imported)
				}
			case strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/"):
				violations = append(violations, path+": Ent store must not import internal packages outside persistence/contracts, got "+imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("Ent store dependency violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestRedisStoresOnlyDependOnContractsAndRedis(t *testing.T) {
	root := filepath.Clean("../persistence/redisstore")
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		imports, err := fileImports(path)
		if err != nil {
			return err
		}
		for _, imported := range imports {
			switch {
			case strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/ent"):
				violations = append(violations, path+": Redis store must not import Ent packages")
			case strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/persistence/redisstore"):
				continue
			case imported == "github.com/srapi/srapi/apps/api/internal/platform/redis":
				continue
			case strings.HasPrefix(imported, moduleImportPrefix):
				importedModule, importedLayer := importedModuleLayer(imported)
				if importedModule == "" || importedLayer != "contract" {
					violations = append(violations, path+": Redis store may only import module contracts, got "+imported)
				}
			case strings.HasPrefix(imported, "github.com/srapi/srapi/apps/api/internal/"):
				violations = append(violations, path+": Redis store must not import internal packages outside persistence/contracts/platform redis, got "+imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("Redis store dependency violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestGoVersionPinsStayAligned(t *testing.T) {
	goMod, err := os.ReadFile(filepath.Clean("../../go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goMod), "go "+requiredGoVersion+"\n") {
		t.Fatalf("go.mod must pin latest approved Go version %s", requiredGoVersion)
	}

	dockerfile, err := os.ReadFile(filepath.Clean("../../Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerfile), "golang:"+requiredGoVersion+"-bookworm") {
		t.Fatalf("Dockerfile build image must align with Go %s", requiredGoVersion)
	}
}

func moduleName(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func moduleLayer(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func importedModuleLayer(imported string) (string, string) {
	rest := strings.TrimPrefix(imported, moduleImportPrefix)
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func fileImports(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	imports := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		imports = append(imports, strings.Trim(spec.Path.Value, `"`))
	}
	return imports, nil
}

func fileLineCount(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		return 0, nil
	}
	lines := strings.Count(string(raw), "\n")
	if raw[len(raw)-1] != '\n' {
		lines++
	}
	return lines, nil
}
