package codequality_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	maxProductionGoFileLines = 2200
	maxProductionFuncLines   = 220
)

var generatedPathFragments = []string{
	string(filepath.Separator) + "ent" + string(filepath.Separator),
	string(filepath.Separator) + "internal" + string(filepath.Separator) + "openapi" + string(filepath.Separator),
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

type fileMode int

const (
	includeTests fileMode = iota
	productionOnly
)

func goFiles(t *testing.T, root string, mode fileMode) []string {
	t.Helper()
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
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no Go files found")
	}
	return files
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

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
