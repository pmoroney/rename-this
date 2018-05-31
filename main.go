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
	newName := []rune{}
	runes := []rune(name)
	w, i := 0, 0 // index of start of word, scan
	for i+1 <= len(runes) {
		eow := false // whether we hit the end of a word
		if i+1 == len(runes) {
			eow = true
		} else if unicode.IsLower(runes[i]) && !unicode.IsLower(runes[i+1]) {
			// lower->non-lower
			eow = true
		}
		i++
		if !eow {
			continue
		}

		// [w,i) is a word.
		newName = append(newName, runes[w])
		w = i
	}
	return strings.ToLower(string(newName))
}

func fixDir(dir string) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		return err
	}
	for _, node := range pkgs {
		ast.Walk(walker(func(n ast.Node) bool {
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
				err := rename.Main(&build.Default, fileOffset, "", new)
				if err == rename.ConflictError {
					log.Printf("Conflict at %s renaming receiver for %s to %s", fileOffset, ident.Name, new)
				} else if err != nil {
					log.Print(err)
				}
			}
			return true
		}), node)
	}
	return nil
}

func main() {
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if path == "." {
			path = filepath.Clean(path)
		}
		fixDir(path)
		return nil
	})
}
