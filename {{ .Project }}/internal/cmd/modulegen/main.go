package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type moduleSpec struct {
	Alias      string
	ImportPath string
	Instance   string
	Arguments  []string
	HTTP       bool
{{- if .Computed.enable_grpc_final }}
	GRPC       bool
{{- end }}
}

func main() {
	modulesDir := flag.String("modules-dir", "internal/modules", "business modules directory")
	appFile := flag.String("app-file", "internal/app/app.go", "file containing Application")
	goMod := flag.String("go-mod", "go.mod", "Go module file")
	output := flag.String("output", "internal/app/modules.gen.go", "generated Go file")
	flag.Parse()
	if err := run(*modulesDir, *appFile, *goMod, *output); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "modulegen: %v\n", err)
		os.Exit(1)
	}
}

func run(modulesDir, appFile, goMod, output string) error {
	modulePath, err := readModulePath(goMod)
	if err != nil {
		return err
	}
	fields, err := applicationFields(appFile)
	if err != nil {
		return err
	}
	modulesRelative, err := filepath.Rel(filepath.Dir(goMod), modulesDir)
	if err != nil {
		return fmt.Errorf("resolve modules directory: %w", err)
	}
	modulesImport := modulePath + "/" + filepath.ToSlash(modulesRelative)
	specs, err := discoverModules(modulesDir, modulesImport, fields)
	if err != nil {
		return err
	}
	source, err := generate(modulePath, specs)
	if err != nil {
		return err
	}
	if err := showDiff(output, source); err != nil {
		return err
	}
	return writeAtomic(output, source)
}

func readModulePath(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read go module: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", errors.New("go module path not found")
}

func applicationFields(path string) (map[string][]string, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse application: %w", err)
	}
	imports, err := importPaths(file)
	if err != nil {
		return nil, err
	}
	fields := make(map[string][]string)
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}
		for _, item := range generic.Specs {
			specification, ok := item.(*ast.TypeSpec)
			if !ok || specification.Name.Name != "Application" {
				continue
			}
			structure, ok := specification.Type.(*ast.StructType)
			if !ok {
				return nil, errors.New("Application must be a struct")
			}
			for _, field := range structure.Fields.List {
				key, err := typeKey(field.Type, imports)
				if err != nil {
					continue
				}
				for _, name := range field.Names {
					fields[key] = append(fields[key], name.Name)
				}
			}
			return fields, nil
		}
	}
	return nil, errors.New("Application struct not found")
}

func discoverModules(dir, rootImport string, application map[string][]string) ([]moduleSpec, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read modules directory: %w", err)
	}
	specs := make([]moduleSpec, 0)
	aliases := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		moduleImport := rootImport + "/" + entry.Name()
		spec, found, err := inspectModule(filepath.Join(dir, entry.Name()), rootImport, moduleImport, application)
		if err != nil {
			return nil, fmt.Errorf("inspect module %s: %w", entry.Name(), err)
		}
		if !found {
			continue
		}
		if spec.Alias == "modules" {
			return nil, fmt.Errorf("module %s uses reserved package name modules", entry.Name())
		}
		if _, exists := aliases[spec.Alias]; exists {
			return nil, fmt.Errorf("module package name %s is duplicated", spec.Alias)
		}
		aliases[spec.Alias] = struct{}{}
		spec.ImportPath = moduleImport
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(left, right int) bool {
		return specs[left].ImportPath < specs[right].ImportPath
	})
	return specs, nil
}

func inspectModule(dir, rootImport, moduleImport string, application map[string][]string) (moduleSpec, bool, error) {
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		return moduleSpec{}, false, err
	}
	for _, currentPackage := range packages {
		implementation := ""
		hasHTTP := false
{{- if .Computed.enable_grpc_final }}
		hasGRPC := false
{{- end }}
		for _, file := range currentPackage.Files {
			imports, err := importPaths(file)
			if err != nil {
				return moduleSpec{}, false, err
			}
			for name, capabilities := range assertedModules(file, imports, rootImport) {
				if implementation != "" && implementation != name {
					return moduleSpec{}, false, errors.New("one module constructor must own the transport capabilities")
				}
				implementation = name
				hasHTTP = hasHTTP || capabilities.HTTP
{{- if .Computed.enable_grpc_final }}
				hasGRPC = hasGRPC || capabilities.GRPC
{{- end }}
			}
		}
		if implementation == "" {
			return moduleSpec{}, false, nil
		}
		fields := application["*"+moduleImport+"."+implementation]
		if len(fields) > 1 {
			return moduleSpec{}, false, fmt.Errorf("module %s matches multiple Application fields", implementation)
		}
		if len(fields) == 1 {
			return moduleSpec{Alias: currentPackage.Name, Instance: "a."+fields[0], HTTP: hasHTTP{{ if .Computed.enable_grpc_final }}, GRPC: hasGRPC{{ end }}}, true, nil
		}
		for _, file := range currentPackage.Files {
			imports, err := importPaths(file)
			if err != nil {
				return moduleSpec{}, false, err
			}
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Recv != nil || function.Name.Name != "New" || !returnsType(function.Type.Results, implementation) {
					continue
				}
				arguments, err := constructorArguments(function.Type.Params, imports, moduleImport, application)
				if err != nil {
					return moduleSpec{}, false, err
				}
				return moduleSpec{Alias: currentPackage.Name, Arguments: arguments, HTTP: hasHTTP{{ if .Computed.enable_grpc_final }}, GRPC: hasGRPC{{ end }}}, true, nil
			}
		}
		return moduleSpec{}, false, errors.New("Module implementation must have a New constructor")
	}
	return moduleSpec{}, false, nil
}

func assertedModules(file *ast.File, imports map[string]string, rootImport string) map[string]moduleSpec {
	found := map[string]moduleSpec{}
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || generic.Tok != token.VAR {
			continue
		}
		for _, item := range generic.Specs {
			specification, ok := item.(*ast.ValueSpec)
			if !ok || len(specification.Names) != 1 || specification.Names[0].Name != "_" || len(specification.Values) != 1 {
				continue
			}
			selector, ok := specification.Type.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "Module" && selector.Sel.Name != "HTTPModule"{{ if .Computed.enable_grpc_final }} && selector.Sel.Name != "GRPCModule"{{ end }}) {
				continue
			}
			alias, ok := selector.X.(*ast.Ident)
			if !ok || imports[alias.Name] != rootImport {
				continue
			}
			if name := assertedType(specification.Values[0]); name != "" {
				item := found[name]
{{- if .Computed.enable_grpc_final }}
				if selector.Sel.Name == "GRPCModule" {
					item.GRPC = true
				} else {
{{- end }}
					item.HTTP = true
{{- if .Computed.enable_grpc_final }}
				}
{{- end }}
				found[name] = item
			}
		}
	}
	return found
}

func assertedType(expression ast.Expr) string {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return ""
	}
	value := call.Fun
	if parenthesized, ok := value.(*ast.ParenExpr); ok {
		value = parenthesized.X
	}
	if pointer, ok := value.(*ast.StarExpr); ok {
		value = pointer.X
	}
	if identifier, ok := value.(*ast.Ident); ok {
		return identifier.Name
	}
	return ""
}

func returnsType(results *ast.FieldList, name string) bool {
	if results == nil || len(results.List) != 1 {
		return false
	}
	value := results.List[0].Type
	if pointer, ok := value.(*ast.StarExpr); ok {
		value = pointer.X
	}
	identifier, ok := value.(*ast.Ident)
	return ok && identifier.Name == name
}

func constructorArguments(parameters *ast.FieldList, imports map[string]string, moduleImport string, application map[string][]string) ([]string, error) {
	if parameters == nil {
		return nil, nil
	}
	arguments := make([]string, 0, len(parameters.List))
	for _, parameter := range parameters.List {
		key, err := typeKey(parameter.Type, imports)
		if err != nil {
			return nil, err
		}
		fields := application[key]
		if len(fields) == 0 {
			pointer := strings.HasPrefix(key, "*")
			name := strings.TrimPrefix(key, "*")
			if !strings.Contains(name, ".") {
				key = moduleImport + "." + name
				if pointer {
					key = "*" + key
				}
				fields = application[key]
			}
		}
		if len(fields) == 0 {
			return nil, fmt.Errorf("constructor dependency %s has no Application field", key)
		}
		if len(fields) > 1 {
			return nil, fmt.Errorf("constructor dependency %s matches multiple Application fields", key)
		}
		count := len(parameter.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			arguments = append(arguments, "a."+fields[0])
		}
	}
	return arguments, nil
}

func importPaths(file *ast.File) (map[string]string, error) {
	imports := make(map[string]string, len(file.Imports))
	for _, item := range file.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("decode import path: %w", err)
		}
		alias := filepath.Base(path)
		if item.Name != nil {
			alias = item.Name.Name
		}
		imports[alias] = path
	}
	return imports, nil
}

func typeKey(expression ast.Expr, imports map[string]string) (string, error) {
	switch value := expression.(type) {
	case *ast.StarExpr:
		key, err := typeKey(value.X, imports)
		return "*" + key, err
	case *ast.SelectorExpr:
		alias, ok := value.X.(*ast.Ident)
		if !ok || imports[alias.Name] == "" {
			return "", errors.New("unsupported selector type")
		}
		return imports[alias.Name] + "." + value.Sel.Name, nil
	case *ast.Ident:
		return value.Name, nil
	default:
		return "", fmt.Errorf("unsupported dependency type %T", expression)
	}
}

func generate(modulePath string, specs []moduleSpec) ([]byte, error) {
	var source bytes.Buffer
	source.WriteString("// Code generated by modulegen; DO NOT EDIT.\n\npackage app\n\nimport (\n")
	fmt.Fprintf(&source, "%q\n", modulePath+"/internal/modules")
	for _, spec := range specs {
		if spec.Instance == "" {
			fmt.Fprintf(&source, "%s %q\n", spec.Alias, spec.ImportPath)
		}
	}
	source.WriteString(")\n\nfunc (a *Application) generatedModules() {{ if .Computed.enable_grpc_final }}([]modules.HTTPModule, []modules.GRPCModule){{ else }}[]modules.HTTPModule{{ end }} {\n")
	source.WriteString("var httpModules []modules.HTTPModule\n{{ if .Computed.enable_grpc_final }}var grpcModules []modules.GRPCModule\n{{ end }}")
	for index, spec := range specs {
		name := fmt.Sprintf("module%d", index)
		if spec.Instance != "" {
			fmt.Fprintf(&source, "%s := %s\n", name, spec.Instance)
		} else {
			fmt.Fprintf(&source, "%s := %s.New(%s)\n", name, spec.Alias, strings.Join(spec.Arguments, ", "))
		}
		if spec.HTTP {
			fmt.Fprintf(&source, "httpModules = append(httpModules,%s)\n", name)
		}
{{- if .Computed.enable_grpc_final }}
		if spec.GRPC {
			fmt.Fprintf(&source, "grpcModules = append(grpcModules,%s)\n", name)
		}
{{- end }}
	}
	source.WriteString("return httpModules{{ if .Computed.enable_grpc_final }},grpcModules{{ end }}\n}\n")
	formatted, err := format.Source(source.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated modules: %w", err)
	}
	return formatted, nil
}

func writeAtomic(output string, source []byte) error {
	current, err := os.ReadFile(output)
	if err == nil && bytes.Equal(current, source) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read generated modules: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(output), ".modules.gen-*.go")
	if err != nil {
		return fmt.Errorf("create generated modules: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(source); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write generated modules: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set generated modules permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close generated modules: %w", err)
	}
	if err := os.Rename(temporaryPath, output); err != nil {
		return fmt.Errorf("replace generated modules: %w", err)
	}
	return nil
}

func showDiff(output string, source []byte) error {
	current, err := os.ReadFile(output)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Printf("create %s\n", output)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read generated modules: %w", err)
	}
	if bytes.Equal(current, source) {
		fmt.Printf("%s unchanged\n", output)
		return nil
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(current)),
		B:        difflib.SplitLines(string(source)),
		FromFile: output,
		ToFile:   output + " (generated)",
		Context:  3,
	})
	if err != nil {
		return fmt.Errorf("render modules diff: %w", err)
	}
	fmt.Print(diff)
	return nil
}
