package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type entry struct {
	pkg  string
	name string
	kind string // type|const|var|func
	code string
}

var quals = map[string]string{} // qualifier -> import path

func nodeText(fset *token.FileSet, n ast.Node) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, n); err != nil {
		panic(err)
	}
	return b.String()
}

func main() {
	pkgs := []struct{ dir, name string }{
		{"internal/meta", "meta"},
		{"internal/util", "util"},
		{"lyrics3", "lyrics3"},
		{"id3v1", "id3v1"},
		{"id3v2", "id3v2"},
		{"vorbis", "vorbis"},
		{"ape", "ape"},
		{"ogg", "ogg"},
		{"flac", "flac"},
		{"mp4", "mp4"},
		{"iff", "iff"},
		{"mpeg", "mpeg"},
		{"dsf", "dsf"},
		{"wavpack", "wavpack"},
		{"mpc", "mpc"},
		{"aac", "aac"},
	}
	seen := map[string]bool{}
	var types, consts, vars_, funcs []entry
	skip := map[string]bool{"init": true}

	for _, p := range pkgs {
		fset := token.NewFileSet()
		des, err := os.ReadDir(p.dir)
		if err != nil {
			panic(err)
		}
		var files []string
		for _, de := range des {
			n := de.Name()
			if !de.IsDir() && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
				files = append(files, filepath.Join(p.dir, n))
			}
		}
		sort.Strings(files)
		for _, fname := range files {
			f, err := parser.ParseFile(fset, fname, nil, parser.ParseComments)
			if err != nil {
				panic(err)
			}
			{
				for _, im := range f.Imports {
					path := strings.Trim(im.Path.Value, "\"")
					q := filepath.Base(path)
					if im.Name != nil {
						q = im.Name.Name
					}
					if q != "." && q != "_" {
						quals[q] = path
					}
				}
				for _, d := range f.Decls {
					switch decl := d.(type) {
					case *ast.GenDecl:
						for _, sp := range decl.Specs {
							switch s := sp.(type) {
							case *ast.TypeSpec:
								if !s.Name.IsExported() || seen[s.Name.Name] {
									continue
								}
								seen[s.Name.Name] = true
								// build alias: type X[params] = pkg.X[names]
								var params, names string
								if s.TypeParams != nil {
									var pp, nn []string
									for _, fld := range s.TypeParams.List {
										ns := []string{}
										for _, n := range fld.Names {
											ns = append(ns, n.Name)
											nn = append(nn, n.Name)
										}
										pp = append(pp, strings.Join(ns, ",")+" "+nodeText(fset, fld.Type))
									}
									params = "[" + strings.Join(pp, ", ") + "]"
									names = "[" + strings.Join(nn, ",") + "]"
								}
								types = append(types, entry{
									p.name, s.Name.Name, "type",
									fmt.Sprintf("type %s%s = %s.%s%s", s.Name.Name, params, p.name, s.Name.Name, names),
								})
							case *ast.ValueSpec:
								for i, n := range s.Names {
									if !n.IsExported() || seen[n.Name] {
										continue
									}
									seen[n.Name] = true
									if decl.Tok == token.CONST {
										consts = append(consts, entry{
											p.name, n.Name, "const",
											fmt.Sprintf("const %s = %s.%s", n.Name, p.name, n.Name),
										})
									} else {
										var typ string
										if s.Type != nil && len(s.Names) == 1 {
											typ = " " + nodeText(fset, s.Type)
										} else if i < len(s.Values) {
											typ = ""
										}
										vars_ = append(vars_, entry{
											p.name, n.Name, "var",
											fmt.Sprintf("var %s%s = %s.%s", n.Name, typ, p.name, n.Name),
										})
									}
								}
							}
						}
					case *ast.FuncDecl:
						if decl.Recv != nil || !decl.Name.IsExported() || skip[decl.Name.Name] || seen[decl.Name.Name] {
							continue
						}
						seen[decl.Name.Name] = true
						// signature: typeparams + params + results
						var tp, targs string
						if decl.Type.TypeParams != nil {
							var pp, nn []string
							for _, fld := range decl.Type.TypeParams.List {
								ns := []string{}
								for _, n := range fld.Names {
									ns = append(ns, n.Name)
									nn = append(nn, n.Name)
								}
								pp = append(pp, strings.Join(ns, ",")+" "+nodeText(fset, fld.Type))
							}
							tp = "[" + strings.Join(pp, ", ") + "]"
							targs = "[" + strings.Join(nn, ",") + "]"
						}
						var params, args []string
						cnt := 0
						nparams := decl.Type.Params.NumFields()
						idx := 0
						for _, fld := range decl.Type.Params.List {
							idx++
							typText := nodeText(fset, fld.Type)
							variadic := strings.HasPrefix(typText, "...")
							ns := []string{}
							if len(fld.Names) == 0 {
								ns = append(ns, fmt.Sprintf("p%d", cnt))
								cnt++
							} else {
								for _, n := range fld.Names {
									ns = append(ns, n.Name)
								}
							}
							for j, n := range ns {
								params = append(params, n+" "+typText)
								if variadic && idx == nparams && j == len(ns)-1 {
									args = append(args, n+"...")
								} else {
									args = append(args, n)
								}
							}
						}
						var res string
						if decl.Type.Results != nil {
							var rr []string
							for _, fld := range decl.Type.Results.List {
								t := nodeText(fset, fld.Type)
								if len(fld.Names) == 0 {
									rr = append(rr, t)
								} else {
									for _, n := range fld.Names {
										rr = append(rr, n.Name+" "+t)
									}
								}
							}
							res = "(" + strings.Join(rr, ", ") + ")"
							if len(rr) == 1 && !strings.Contains(rr[0], " ") {
								res = rr[0]
							}
						}
						call := fmt.Sprintf("%s.%s%s(%s)", p.name, decl.Name.Name, targs, strings.Join(args, ", "))
						body := call
						if res != "" {
							body = "return " + call
						}
						funcs = append(funcs, entry{
							p.name, decl.Name.Name, "func",
							fmt.Sprintf("func %s%s(%s) %s {\n\t%s\n}", decl.Name.Name, tp, strings.Join(params, ", "), res, body),
						})
					}
				}
			}
		}
	}

	var out bytes.Buffer
	var full strings.Builder
	for _, list := range [][]entry{types, consts, vars_, funcs} {
		for _, e := range list {
			full.WriteString(e.code + "\n")
		}
	}

	out.WriteString("package lofty\n\n// Code generated by scripts/facadegen; DO NOT EDIT.\n// Facade re-exports for API compatibility after the internal split.\n\nimport (\n")
	var iq []string
	for q, path := range quals {
		if q == "meta" || q == "util" || q == "lyrics3" || q == "id3v1" || q == "id3v2" || q == "vorbis" || q == "ape" || q == "ogg" || q == "flac" || q == "mp4" || q == "iff" || q == "mpeg" || q == "dsf" || q == "wavpack" || q == "mpc" || q == "aac" {
			continue
		}
		if strings.Contains(full.String(), q+".") {
			iq = append(iq, q+"\t"+path)
		}
	}
	sort.Strings(iq)
	emitImp := func(qual, path string) {
		if qual == filepath.Base(path) {
			fmt.Fprintf(&out, "\t\"%s\"\n", path)
		} else {
			fmt.Fprintf(&out, "\t%s \"%s\"\n", qual, path)
		}
	}
	// group 1: stdlib; group 2: third-party; group 3: module-local
	var std, third []string
	for _, e := range iq {
		parts := strings.SplitN(e, "\t", 2)
		if strings.Contains(strings.SplitN(parts[1], "/", 2)[0], ".") {
			third = append(third, e)
		} else {
			std = append(std, e)
		}
	}
	for _, e := range std {
		parts := strings.SplitN(e, "\t", 2)
		emitImp(parts[0], parts[1])
	}
	if len(std) > 0 && len(third) > 0 {
		out.WriteString("\n")
	}
	for _, e := range third {
		parts := strings.SplitN(e, "\t", 2)
		emitImp(parts[0], parts[1])
	}
	if len(std)+len(third) > 0 && len(pkgs) > 0 {
		out.WriteString("\n")
	}
	for _, p := range pkgs {
		fmt.Fprintf(&out, "\t%s \"github.com/subframe7536/lofty-go/%s\"\n", p.name, p.dir)
	}
	out.WriteString(")\n")

	emit := func(title string, list []entry, block string) {
		out.WriteString("\n// --- " + title + " ---\n\n")
		byPkg := map[string][]entry{}
		var order []string
		for _, e := range list {
			if _, ok := byPkg[e.pkg]; !ok {
				order = append(order, e.pkg)
			}
			byPkg[e.pkg] = append(byPkg[e.pkg], e)
		}
		for _, k := range order {
			out.WriteString("// from " + k + "\n")
			if block == "" {
				for _, e := range byPkg[k] {
					out.WriteString(e.code + "\n\n")
				}
				continue
			}
			out.WriteString(block + " (\n")
			for _, e := range byPkg[k] {
				out.WriteString("\t" + strings.TrimPrefix(e.code, block+" ") + "\n")
			}
			out.WriteString(")\n")
		}
	}
	emit("types", types, "type")
	emit("consts", consts, "const")
	emit("vars", vars_, "var")
	emit("funcs", funcs, "")

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		fmt.Fprintln(os.Stderr, "format error:", err)
		if werr := os.WriteFile("facade_gen.go.bad", out.Bytes(), 0o644); werr != nil {
			fmt.Fprintln(os.Stderr, werr)
		}
		return
	}
	if err := os.WriteFile("facade_gen.go", formatted, 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote facade_gen.go:", len(types), "types,", len(consts), "consts,", len(vars_), "vars,", len(funcs), "funcs")
}
