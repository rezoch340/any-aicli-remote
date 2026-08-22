// Command naming-gate rejects compressed declared names in the Go backend.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var permittedShortTechnicalNames = map[string]struct{}{
	"AI": {},
	"DB": {},
	"FS": {},
	"ID": {},
	"IO": {},
	"IP": {},
	"OK": {},
	"OS": {},
	"UI": {},
	"WS": {},
}

var forbiddenAbbreviations = map[string]struct{}{
	"abs": {}, "addr": {}, "addrs": {}, "arg": {}, "args": {},
	"buf": {}, "bufs": {}, "cfg": {}, "ch": {}, "cmd": {}, "cmds": {},
	"conn": {}, "conns": {}, "ctx": {}, "cwd": {}, "dir": {}, "dirs": {},
	"dst": {}, "env": {}, "err": {}, "errs": {}, "fut": {}, "futs": {},
	"idx": {}, "iface": {}, "ifaces": {}, "msg": {}, "msgs": {}, "mtime": {}, "mu": {}, "nid": {}, "num": {}, "nums": {},
	"ops": {}, "opts": {}, "pid": {}, "pids": {}, "proc": {}, "procs": {},
	"rel": {}, "req": {}, "reqs": {}, "res": {}, "resp": {}, "resps": {},
	"rid": {}, "sha": {}, "sid": {}, "sids": {}, "spec": {}, "specs": {}, "src": {},
	"str": {}, "strs": {}, "tid": {}, "tmp": {},
}

type namingViolation struct {
	position token.Position
	name     string
	reason   string
}

func main() {
	scanDirectory := "backend"
	if len(os.Args) > 1 {
		scanDirectory = os.Args[1]
	}
	absoluteScanDirectory, pathError := filepath.Abs(scanDirectory)
	if pathError != nil {
		fmt.Fprintln(os.Stderr, pathError)
		os.Exit(2)
	}

	fileSet := token.NewFileSet()
	violations := make([]namingViolation, 0)
	walkError := filepath.WalkDir(absoluteScanDirectory, func(filePath string, entry fs.DirEntry, traversalError error) error {
		if traversalError != nil {
			return traversalError
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(filePath) != ".go" {
			return nil
		}
		syntaxTree, parseError := parser.ParseFile(fileSet, filePath, nil, parser.SkipObjectResolution)
		if parseError != nil {
			return parseError
		}
		ast.Inspect(syntaxTree, func(syntaxNode ast.Node) bool {
			switch declaration := syntaxNode.(type) {
			case *ast.FuncDecl:
				appendViolation(fileSet, declaration.Name, &violations)
			case *ast.TypeSpec:
				appendViolation(fileSet, declaration.Name, &violations)
			case *ast.Field:
				for _, identifier := range declaration.Names {
					appendViolation(fileSet, identifier, &violations)
				}
			case *ast.ValueSpec:
				for _, identifier := range declaration.Names {
					appendViolation(fileSet, identifier, &violations)
				}
			case *ast.AssignStmt:
				if declaration.Tok == token.DEFINE {
					for _, expression := range declaration.Lhs {
						appendDeclaredExpression(fileSet, expression, &violations)
					}
				}
			case *ast.RangeStmt:
				if declaration.Tok == token.DEFINE {
					appendDeclaredExpression(fileSet, declaration.Key, &violations)
					appendDeclaredExpression(fileSet, declaration.Value, &violations)
				}
			}
			return true
		})
		return nil
	})
	if walkError != nil {
		fmt.Fprintln(os.Stderr, walkError)
		os.Exit(2)
	}

	sort.Slice(violations, func(firstIndex, secondIndex int) bool {
		firstPosition := violations[firstIndex].position
		secondPosition := violations[secondIndex].position
		if firstPosition.Filename != secondPosition.Filename {
			return firstPosition.Filename < secondPosition.Filename
		}
		if firstPosition.Line != secondPosition.Line {
			return firstPosition.Line < secondPosition.Line
		}
		return firstPosition.Column < secondPosition.Column
	})
	for _, violation := range violations {
		relativePath, relativeError := filepath.Rel(absoluteScanDirectory, violation.position.Filename)
		if relativeError != nil {
			relativePath = violation.position.Filename
		}
		fmt.Fprintf(os.Stderr, "%s:%d:%d: %q failed naming gate: %s\n", relativePath, violation.position.Line, violation.position.Column, violation.name, violation.reason)
	}
	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr, "naming gate failed with %d violation(s)\n", len(violations))
		os.Exit(1)
	}
	fmt.Println("naming gate passed")
}

func appendDeclaredExpression(fileSet *token.FileSet, expression ast.Expr, violations *[]namingViolation) {
	if expression == nil {
		return
	}
	switch declaration := expression.(type) {
	case *ast.Ident:
		appendViolation(fileSet, declaration, violations)
	case *ast.ParenExpr:
		appendDeclaredExpression(fileSet, declaration.X, violations)
	}
}

func appendViolation(fileSet *token.FileSet, identifier *ast.Ident, violations *[]namingViolation) {
	if identifier == nil || identifier.Name == "_" {
		return
	}
	if utf8.RuneCountInString(identifier.Name) < 3 {
		if _, permitted := permittedShortTechnicalNames[identifier.Name]; !permitted {
			*violations = append(*violations, namingViolation{
				position: fileSet.Position(identifier.Pos()),
				name:     identifier.Name,
				reason:   "declared names shorter than three characters are forbidden",
			})
			return
		}
	}
	lowerIdentifier := strings.ToLower(identifier.Name)
	if _, forbidden := forbiddenAbbreviations[lowerIdentifier]; forbidden {
		*violations = append(*violations, namingViolation{
			position: fileSet.Position(identifier.Pos()),
			name:     identifier.Name,
			reason:   "abbreviations must be replaced with complete words",
		})
		return
	}
	for _, word := range splitIdentifier(identifier.Name) {
		if _, forbidden := forbiddenAbbreviations[strings.ToLower(word)]; forbidden {
			*violations = append(*violations, namingViolation{
				position: fileSet.Position(identifier.Pos()),
				name:     identifier.Name,
				reason:   fmt.Sprintf("contains forbidden abbreviation %q", word),
			})
			return
		}
	}
}

func splitIdentifier(identifier string) []string {
	runes := []rune(strings.ReplaceAll(identifier, "_", " "))
	words := make([]string, 0, 4)
	wordStart := 0
	flushWord := func(wordEnd int) {
		if wordEnd > wordStart {
			word := strings.TrimSpace(string(runes[wordStart:wordEnd]))
			if word != "" {
				words = append(words, word)
			}
		}
		wordStart = wordEnd
	}
	for runeIndex, currentRune := range runes {
		if unicode.IsSpace(currentRune) {
			flushWord(runeIndex)
			wordStart = runeIndex + 1
			continue
		}
		if runeIndex == wordStart {
			continue
		}
		previousRune := runes[runeIndex-1]
		nextIsLowercase := runeIndex+1 < len(runes) && unicode.IsLower(runes[runeIndex+1])
		caseBoundary := unicode.IsUpper(currentRune) && (unicode.IsLower(previousRune) || unicode.IsDigit(previousRune) || (unicode.IsUpper(previousRune) && nextIsLowercase))
		digitBoundary := unicode.IsDigit(currentRune) != unicode.IsDigit(previousRune) && (unicode.IsDigit(currentRune) || unicode.IsDigit(previousRune))
		if caseBoundary || digitBoundary {
			flushWord(runeIndex)
		}
	}
	flushWord(len(runes))
	return words
}
