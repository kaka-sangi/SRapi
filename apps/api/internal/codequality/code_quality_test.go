//go:build codequality

package codequality_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

const (
	maxProductionGoFileLines = 2180
	maxProductionFuncLines   = 210
)

var generatedPathFragments = []string{
	string(filepath.Separator) + "ent" + string(filepath.Separator),
	string(filepath.Separator) + "internal" + string(filepath.Separator) + "openapi" + string(filepath.Separator),
	string(filepath.Separator) + "packages" + string(filepath.Separator) + "sdk" + string(filepath.Separator) + "typescript" + string(filepath.Separator) + "src" + string(filepath.Separator),
}

func TestGoFilesAreGofmtFormatted(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root, includeTests)
	args := append([]string{"-l"}, files...)
	cmd := exec.Command("gofmt", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run gofmt -l: %v", err)
	}
	out = bytes.TrimSpace(out)
	if len(out) > 0 {
		t.Fatalf("gofmt drift detected:\n%s", out)
	}
}

func TestGoVetPasses(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = filepath.Join(root, "apps", "api")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet ./... failed: %v\n%s", err, out)
	}
}

func TestGitDiffCheckPasses(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("git", "diff", "--check")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git diff --check failed: %v\n%s", err, out)
	}
}

func TestMakeCheckRunsMandatoryQualityGates(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	dependencies := makeTargetDependencies(string(raw), "check")
	required := []string{
		"diff-check",
		"architecture-check",
		"code-quality-check",
		"api-test",
		"secret-scan",
		"openapi-codegen-check",
		"openapi-ts-codegen-check",
		"ent-generate-check",
		"migration-check",
		"observability-rules-check",
	}
	var missing []string
	for _, target := range required {
		if !dependencies[target] {
			missing = append(missing, target)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("make check is missing mandatory quality gates: %s", strings.Join(missing, ", "))
	}
}

func TestBootstrapEnvGeneratesLocalSecrets(t *testing.T) {
	root := repoRoot(t)
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	devPs1, err := os.ReadFile(filepath.Join(root, "tools", "dev.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(root, "tools", "bootstrap-env.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name    string
		content string
		want    string
	}{
		{"Makefile", string(makefile), "BOOTSTRAP_ENV ?= node tools/bootstrap-env.mjs"},
		{"Makefile", string(makefile), "ENV_CHECK ?= node tools/env-check.mjs"},
		{"Makefile", string(makefile), "$(BOOTSTRAP_ENV)"},
		{"Makefile", string(makefile), "$(ENV_CHECK)"},
		{"tools/dev.ps1", string(devPs1), "tools/bootstrap-env.mjs"},
		{"tools/dev.ps1", string(devPs1), "\"env-check\""},
		{"tools/dev.ps1", string(devPs1), "Invoke-Step \"make\" @(\"env-check\")"},
		{"tools/bootstrap-env.mjs", string(script), "randomBytes(32)"},
		{"tools/bootstrap-env.mjs", string(script), "writeFileSync(envPath, output, { mode: 0o600, flag: \"wx\" })"},
	}
	for _, check := range checks {
		if !strings.Contains(check.content, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}
}

func TestDeployPreflightIsExposed(t *testing.T) {
	root := repoRoot(t)
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	devPs1, err := os.ReadFile(filepath.Join(root, "tools", "dev.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(root, "tools", "deploy-preflight.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name    string
		content string
		want    string
	}{
		{"Makefile", string(makefile), "DEPLOY_PREFLIGHT ?= node tools/deploy-preflight.mjs"},
		{"Makefile", string(makefile), "deploy-preflight:"},
		{"Makefile", string(makefile), "$(DEPLOY_PREFLIGHT)"},
		{"tools/dev.ps1", string(devPs1), "\"deploy-preflight\""},
		{"tools/dev.ps1", string(devPs1), "Invoke-Step \"make\" @(\"deploy-preflight\")"},
		{"tools/deploy-preflight.mjs", string(script), "checkEnvFile"},
		{"tools/deploy-preflight.mjs", string(script), "tools/observability-rules-check.mjs"},
		{"tools/deploy-preflight.mjs", string(script), "Docker Compose command not found"},
		{"tools/deploy-preflight.mjs", string(script), "SRAPI_DEPLOY_PREFLIGHT_STRICT_TOOLS"},
	}
	for _, check := range checks {
		if !strings.Contains(check.content, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}
}

func TestMigrationWorkflowTargetsArePinned(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	required := []string{
		"ATLAS ?= npx --yes @ariga/atlas@1.2.0",
		"migration-diff:",
		"migration-hash:",
		"grep -Eq '^[0-9]{6}_[a-z0-9_]+$$'",
		"$(ATLAS) migrate diff \"$(MIGRATION_NAME)\" --env local",
		"mv \"$(API_DIR)/migrations/postgres/up/$$new\" \"$(API_DIR)/migrations/postgres/up/$(MIGRATION_NAME).sql\"",
		"$(ATLAS) migrate hash --dir file://migrations/postgres/up",
	}
	var missing []string
	for _, needle := range required {
		if !strings.Contains(content, needle) {
			missing = append(missing, needle)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("migration workflow Makefile entries missing or unpinned:\n%s", strings.Join(missing, "\n"))
	}

	atlas, err := os.ReadFile(filepath.Join(root, "apps", "api", "atlas.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	atlasContent := string(atlas)
	for _, needle := range []string{
		`src = "ent://ent/schema"`,
		`dev = "docker://postgres/16/dev?search_path=public"`,
		`dir = "file://migrations/postgres/up"`,
	} {
		if !strings.Contains(atlasContent, needle) {
			t.Fatalf("apps/api/atlas.hcl missing %q", needle)
		}
	}
}

func TestSecretScanCoversGeneratedContractsAndLockfiles(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, ".secretlintignore"))
	if err != nil {
		t.Fatal(err)
	}
	scannedPaths := []string{
		"package-lock.json",
		"build/openapi/openapi.bundle.yaml",
		"apps/api/internal/openapi/openapi.gen.go",
		"packages/sdk/typescript/src/index.ts",
	}
	var violations []string
	for _, pattern := range secretlintIgnorePatterns(string(raw)) {
		for _, path := range scannedPaths {
			if ignorePatternCovers(pattern, path) {
				violations = append(violations, pattern+" hides "+path)
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("secret scan must cover generated contracts and lockfiles:\n%s", strings.Join(violations, "\n"))
	}
}

func TestProductionGoFilesStaySmallEnough(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root, productionOnly)
	var violations []string
	for _, path := range files {
		lines := lineCount(t, path)
		if lines > maxProductionGoFileLines {
			violations = append(violations, relativePath(root, path)+": production Go file has "+strconv.Itoa(lines)+" lines; split by ownership before adding more code")
		}
	}
	if len(violations) > 0 {
		t.Fatalf("production file size violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestProductionFunctionsStaySmallEnough(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root, productionOnly)
	var violations []string
	for _, path := range files {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			start := fileSet.Position(fn.Pos()).Line
			end := fileSet.Position(fn.End()).Line
			lines := end - start + 1
			if lines > maxProductionFuncLines {
				violations = append(violations, relativePath(root, path)+":"+strconv.Itoa(start)+": "+fn.Name.Name+" has "+strconv.Itoa(lines)+" lines; extract helpers before growing it further")
			}
			return true
		})
	}
	if len(violations) > 0 {
		t.Fatalf("production function size violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestRepositoryTextFilesAreClean(t *testing.T) {
	root := repoRoot(t)
	files := repoFiles(t, root)
	var violations []string
	for _, rel := range files {
		if !isTextHygieneFile(rel) || isGeneratedPath(filepath.Join(root, rel)) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		violations = append(violations, textHygieneViolations(rel, raw)...)
	}
	if len(violations) > 0 {
		t.Fatalf("text hygiene violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestNodeScriptsParse(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range repoFiles(t, root) {
		if filepath.Ext(rel) != ".mjs" && filepath.Ext(rel) != ".js" {
			continue
		}
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			runCommand(t, root, "node", "--check", rel)
		})
	}
}

func TestNodeUnitTestsPass(t *testing.T) {
	root := repoRoot(t)
	var files []string
	for _, rel := range repoFiles(t, root) {
		if strings.HasSuffix(rel, ".test.mjs") {
			files = append(files, rel)
		}
	}
	if len(files) == 0 {
		return
	}
	args := append([]string{"--test"}, files...)
	runCommand(t, root, "node", args...)
}

func TestShellScriptsParse(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range repoFiles(t, root) {
		if filepath.Ext(rel) != ".sh" {
			continue
		}
		runCommand(t, root, "bash", "-n", rel)
	}
}

func TestContainerFilesUsePinnedNonRootDefaults(t *testing.T) {
	root := repoRoot(t)
	var violations []string
	for _, rel := range repoFiles(t, root) {
		switch filepath.Base(rel) {
		case "Dockerfile":
			violations = append(violations, dockerfileViolations(t, root, rel)...)
		}
		if isComposeFile(rel) {
			violations = append(violations, composeImageViolations(t, root, rel)...)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("container configuration violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestProductionCodeHasNoSpeculativeMarkers(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root, productionOnly)
	markers := []string{"TODO", "FIXME", "HACK", "XXX"}
	var violations []string
	for _, path := range files {
		lines := fileLines(t, path)
		for i, line := range lines {
			for _, marker := range markers {
				if strings.Contains(line, marker) {
					violations = append(violations, relativePath(root, path)+":"+strconv.Itoa(i+1)+": remove speculative marker "+marker)
				}
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("production code marker violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestProductionGoAvoidsPanicAndRecoverOutsideBootstrap(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root, productionOnly)
	allowed := map[string][]string{
		"apps/api/internal/app/app.go":           {"recover()"},
		"apps/api/internal/httpserver/server.go": {"panic(err)"},
		// embed.go's fs.Sub only fails on an invalid path, and the //go:embed
		// pattern is a compile-time constant, so this panic is unreachable.
		"apps/api/migrations/embed.go": {"panic(err)"},
	}
	var violations []string
	for _, path := range files {
		rel := relativePath(root, path)
		lines := fileLines(t, path)
		for i, line := range lines {
			for _, call := range []string{"panic(", "recover("} {
				if strings.Contains(line, call) && !allowedEscapeHatch(allowed[rel], line) {
					violations = append(violations, rel+":"+strconv.Itoa(i+1)+": "+call+" must stay out of production code outside documented bootstrap escape hatches")
				}
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("panic/recover violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestProductionGoHasNoPlaceholderNotImplementedHandlers(t *testing.T) {
	root := repoRoot(t)
	files := goFiles(t, root, productionOnly)
	disallowed := []string{
		"http.StatusNotImplemented",
		"StatusNotImplemented",
		`"not_implemented"`,
		`"not implemented yet"`,
	}
	var violations []string
	for _, path := range files {
		rel := relativePath(root, path)
		lines := fileLines(t, path)
		for i, line := range lines {
			for _, marker := range disallowed {
				if strings.Contains(line, marker) {
					violations = append(violations, rel+":"+strconv.Itoa(i+1)+": remove placeholder not-implemented response "+marker)
				}
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("not-implemented placeholder violations:\n%s", strings.Join(violations, "\n"))
	}
}

type fileMode int

const (
	includeTests fileMode = iota
	productionOnly
)

var (
	repoFilesOnce  sync.Once
	repoFilesCache []string
	repoFilesErr   error

	goFilesOnce  [2]sync.Once
	goFilesCache [2][]string
	goFilesErr   [2]error
)

func goFiles(t *testing.T, root string, mode fileMode) []string {
	t.Helper()
	goFilesOnce[mode].Do(func() {
		goFilesCache[mode], goFilesErr[mode] = loadGoFiles(root, mode)
	})
	if goFilesErr[mode] != nil {
		t.Fatal(goFilesErr[mode])
	}
	return append([]string(nil), goFilesCache[mode]...)
}

func loadGoFiles(root string, mode fileMode) ([]string, error) {
	apiRoot := filepath.Join(root, "apps", "api")
	var files []string
	err := filepath.WalkDir(apiRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if mode == productionOnly {
			if strings.HasSuffix(path, "_test.go") || isGeneratedPath(path) {
				return nil
			}
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Go files found")
	}
	return files, nil
}

func repoFiles(t *testing.T, root string) []string {
	t.Helper()
	repoFilesOnce.Do(func() {
		repoFilesCache, repoFilesErr = loadRepoFiles(root)
	})
	if repoFilesErr != nil {
		t.Fatalf("list repository files: %v", repoFilesErr)
	}
	return append([]string(nil), repoFilesCache...)
}

func loadRepoFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		if line != "" {
			files = append(files, filepath.Clean(line))
		}
	}
	return files, nil
}

func isGeneratedPath(path string) bool {
	clean := filepath.Clean(path)
	for _, fragment := range generatedPathFragments {
		if strings.Contains(clean, fragment) {
			return true
		}
	}
	return false
}

func isTextHygieneFile(path string) bool {
	switch filepath.Base(path) {
	case "Makefile", "Dockerfile", ".secretlintignore":
		return true
	}
	switch filepath.Ext(path) {
	case ".go", ".hcl", ".md", ".yaml", ".yml", ".json", ".mjs", ".js", ".ts", ".sh", ".ps1", ".html":
		return true
	default:
		return false
	}
}

func textHygieneViolations(path string, raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var violations []string
	if !utf8.Valid(raw) {
		violations = append(violations, path+": file must be valid UTF-8")
	}
	if raw[len(raw)-1] != '\n' {
		violations = append(violations, path+": file must end with a newline")
	}
	lines := bytes.Split(raw, []byte{'\n'})
	for i, line := range lines {
		if i == len(lines)-1 && len(line) == 0 {
			continue
		}
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if bytes.HasSuffix(line, []byte{' '}) || bytes.HasSuffix(line, []byte{'\t'}) {
			violations = append(violations, path+":"+strconv.Itoa(i+1)+": trailing whitespace")
		}
	}
	return violations
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "apps", "api", "go.mod")); err != nil {
		t.Fatalf("cannot locate repo root from %s: %v", file, err)
	}
	return root
}

func lineCount(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		return 0
	}
	lines := bytes.Count(raw, []byte{'\n'})
	if raw[len(raw)-1] != '\n' {
		lines++
	}
	return lines
}

func fileLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(raw), "\n")
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

func makeTargetDependencies(content, target string) map[string]bool {
	dependencies := map[string]bool{}
	prefix := target + ":"
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		for _, field := range strings.Fields(strings.TrimPrefix(line, prefix)) {
			dependencies[field] = true
		}
		return dependencies
	}
	return dependencies
}

func secretlintIgnorePatterns(content string) []string {
	var patterns []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func ignorePatternCovers(pattern, path string) bool {
	pattern = strings.TrimSuffix(filepath.ToSlash(pattern), "/")
	pattern = strings.TrimSuffix(pattern, "/**")
	path = filepath.ToSlash(path)
	return pattern == path || strings.HasPrefix(path, pattern+"/")
}

func runCommand(t *testing.T, root string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func dockerfileViolations(t *testing.T, root, rel string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	var violations []string
	if strings.Contains(content, ":latest") {
		violations = append(violations, rel+": image tags must be pinned; do not use latest")
	}
	// Require dropping root. Accept the distroless "nonroot" user and the
	// official node image's built-in unprivileged "node" user.
	if !strings.Contains(content, "USER nonroot") && !strings.Contains(content, "USER node") {
		violations = append(violations, rel+": runtime image must declare a non-root user")
	}
	return violations
}

func composeImageViolations(t *testing.T, root, rel string) []string {
	t.Helper()
	lines := fileLines(t, filepath.Join(root, rel))
	var violations []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "image:") {
			continue
		}
		image := strings.TrimSpace(strings.TrimPrefix(trimmed, "image:"))
		if image == "" || strings.Contains(image, "${") {
			continue
		}
		if !strings.Contains(image, ":") || strings.HasSuffix(image, ":latest") {
			violations = append(violations, fmt.Sprintf("%s:%d: image must use an explicit non-latest tag", rel, i+1))
		}
	}
	return violations
}

func isComposeFile(path string) bool {
	base := filepath.Base(path)
	return base == "docker-compose.yml" || base == "docker-compose.yaml" || strings.Contains(base, "compose.")
}

func allowedEscapeHatch(allowed []string, line string) bool {
	for _, snippet := range allowed {
		if strings.Contains(line, snippet) {
			return true
		}
	}
	return false
}
