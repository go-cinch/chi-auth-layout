package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type sourcePackage struct {
	directory    string
	imports      map[string]string
	dependencies map[string]*sourcePackage
	name         string
	funcs        map[string]*ast.FuncDecl
	types        map[string]ast.Expr
	errors       map[string]string
	receivers    map[string]string
}

func loadPackages(root string) ([]*sourcePackage, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("read source directory: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("source path is not a directory")
	}
	var packages []*sourcePackage
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() || path == root {
			return nil
		}
		if filepath.Clean(path) == filepath.Join(filepath.Clean(root), "docs") {
			return filepath.SkipDir
		}
		set := token.NewFileSet()
		parsed, parseErr := parser.ParseDir(set, path, func(info os.FileInfo) bool {
			return strings.HasSuffix(info.Name(), ".go") &&
				!strings.HasSuffix(info.Name(), "_test.go") && !strings.HasSuffix(info.Name(), ".gen.go")
		}, parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %q: %w", path, parseErr)
		}
		for _, parsedPackage := range parsed {
			pkg := &sourcePackage{
				directory: path, imports: make(map[string]string), dependencies: make(map[string]*sourcePackage),
				name: parsedPackage.Name, funcs: make(map[string]*ast.FuncDecl),
				types: make(map[string]ast.Expr), errors: make(map[string]string),
				receivers: make(map[string]string),
			}
			for _, file := range parsedPackage.Files {
				pkg.collect(file)
			}
			packages = append(packages, pkg)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Resolve only packages present in this source tree. External library types
	// remain opaque, while shared local response contracts retain their schemas.
	for _, pkg := range packages {
		for alias, imported := range pkg.imports {
			for _, candidate := range packages {
				relative, err := filepath.Rel(root, candidate.directory)
				if err == nil && strings.HasSuffix(imported, "/"+filepath.ToSlash(relative)) {
					pkg.dependencies[alias] = candidate
					break
				}
			}
		}
	}
	return packages, nil
}

func (p *sourcePackage) collect(file *ast.File) {
	if p.imports == nil {
		p.imports = make(map[string]string)
	}
	for _, item := range file.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			continue
		}
		alias := filepath.Base(path)
		if item.Name != nil {
			alias = item.Name.Name
		}
		if alias != "_" && alias != "." {
			p.imports[alias] = path
		}
	}

	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			receiver := receiverName(value.Recv)
			p.funcs[receiver+"."+value.Name.Name] = value
			if receiver != "" && value.Name.Name == "Name" {
				if name := returnedString(value); name != "" {
					p.receivers[receiver] = name
				}
			}
		case *ast.GenDecl:
			for _, item := range value.Specs {
				switch specification := item.(type) {
				case *ast.TypeSpec:
					p.types[specification.Name.Name] = specification.Type
				case *ast.ValueSpec:
					p.collectError(specification)
				}
			}
		}
	}
}

func (p *sourcePackage) collectError(value *ast.ValueSpec) {
	for index, name := range value.Names {
		if index >= len(value.Values) {
			continue
		}
		call, ok := value.Values[index].(*ast.CallExpr)
		if !ok || (len(call.Args) != 1 && len(call.Args) != 2) {
			continue
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		message, messageOK := stringLiteral(call.Args[len(call.Args)-1])
		if ok && selector.Sel.Name == "New" && messageOK {
			p.errors[name.Name] = message
		}
	}
}

func receiverName(list *ast.FieldList) string {
	if list == nil || len(list.List) == 0 {
		return ""
	}
	expression := list.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func returnedString(function *ast.FuncDecl) string {
	if function.Body == nil {
		return ""
	}
	for _, statement := range function.Body.List {
		if returned, ok := statement.(*ast.ReturnStmt); ok && len(returned.Results) == 1 {
			value, _ := stringLiteral(returned.Results[0])
			return value
		}
	}
	return ""
}

func returnedBool(function *ast.FuncDecl) bool {
	if function == nil || function.Body == nil {
		return false
	}
	for _, statement := range function.Body.List {
		if returned, ok := statement.(*ast.ReturnStmt); ok && len(returned.Results) == 1 {
			value, ok := returned.Results[0].(*ast.Ident)
			return ok && value.Name == "true"
		}
	}
	return false
}

func stringLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func functionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.Ident:
		return value.Name
	default:
		return ""
	}
}
