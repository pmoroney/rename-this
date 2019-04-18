package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/tools/refactor/rename"
)

// walker adapts a function to satisfy the ast.Visitor interface.
// The function return whether the walk should proceed into the node's children.
type walker func(ast.Node) bool

func (w walker) Visit(node ast.Node) ast.Visitor {
	if w(node) {
		return w
	}
	return nil
}

func newName(name string) string {
	overrides := map[string]string{
		"Spotlight": "sl",
		"YBCache":   "yc",
	}
	if ans, ok := overrides[name]; ok {
		return ans
	}
	allUpper := true
	for _, r := range name {
		if !unicode.IsUpper(r) {
			allUpper = false
			break
		}
	}
	if allUpper {
		return strings.ToLower(name)
	}
	allLower := true
	for _, r := range name {
		if !unicode.IsLower(r) {
			allLower = false
			break
		}
	}
	if allLower {
		if len(name) < 3 {
			return name[:1]
		}
		return name[:3]
	}

	// Split camelCase at any lower->upper transition, and split on underscores.
	// Check each word for common initialisms.
	runes := []rune(name)
	plural := false
	if runes[len(runes)-1] == 's' {
		plural = true
		runes = runes[:len(runes)-1]
	}
	newName := []rune{runes[0]}
	i := 1 // index of start of word, scan
	for i+1 <= len(runes) {
		if !unicode.IsLower(runes[i]) {
			newName = append(newName, runes[i])
		}

		i++
	}
	if plural {
		newName = append(newName, 's')
	}
	return strings.ToLower(string(newName))
}

func fixDir(dir string) (bool, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		return false, err
	}
	success := false
	for _, node := range pkgs {
		ast.Walk(walker(func(n ast.Node) bool {
			if success {
				return false
			}
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				return true
			}
			names := fn.Recv.List[0].Names
			if len(names) < 1 {
				return true
			}
			name := names[0].Name
			if name == "this" || name == "self" {
				position := fset.Position(names[0].Pos())
				fileOffset := fmt.Sprintf("%s:#%d", position.Filename, position.Offset)

				typ := fn.Recv.List[0].Type
				star, ok := typ.(*ast.StarExpr)
				if ok {
					typ = star.X
				}

				ident, ok := typ.(*ast.Ident)
				if !ok {
					log.Printf("type not an object or *object: %v", position)
					return true
				}
				new := newName(ident.Name)
				log.Printf("Renaming %#v for %s to %s", position, ident.Name, new)
				err := rename.Main(&build.Default, fileOffset, "", new)
				if err == rename.ConflictError {
					log.Fatalf("Conflict at %s renaming receiver for %s to %s", fileOffset, ident.Name, new)
				} else if err != nil {
					log.Print(err)
				}
				success = true
				return false
			}
			return true
		}), node)
	}
	return success, nil
}

func main() {
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if path == "." {
			path = filepath.Clean(path)
		}
		success := true
		for success {
			success, err = fixDir(path)
			if err != nil {
				log.Fatal(err)
			}
		}
		return nil
	})
}
