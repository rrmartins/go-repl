package main

import (
	"bufio";
	"bytes";
	"container/vector";
	"fmt";
	"go/ast";
	"go/parser";
	"go/printer";
	"io";
	"io/ioutil";
	"os";
	"strings";
)

type World struct {
	pkgs	*vector.StringVector;
	defs	*vector.StringVector;
	code	*vector.Vector;
	exec	string;
}

const TEMPPATH = "/tmp/gorepl"


var (
	bin	= os.Getenv("GOBIN");
	arch	= map[string]string{
		"amd64": "6",
		"386": "8",
		"arm": "5",
	}[os.Getenv("GOARCH")];
)

func (self *World) source() string {
	source := "package main\n";

	for _, v := range self.pkgs.Data() {
		source += "import \"" + v + "\"\n"
	}

	source += "\n";

	for _, d := range self.defs.Data() {
		source += d + "\n\n"
	}

	source += "func noop(_ interface{}) {}\n\n";

	source += "func main() {\n";

	for _, c := range self.code.Data() {
		str := new(bytes.Buffer);
		printer.Fprint(str, c);

		source += "\t" + str.String() + ";\n";
		switch c.(type) {
		case *ast.AssignStmt:
			for _, name := range c.(*ast.AssignStmt).Lhs {
				str := new(bytes.Buffer);
				printer.Fprint(str, name);
				source += "\t" + "noop(" + str.String() + ");\n";
			}
		}
	}

	if self.exec != "" {
		source += "\t" + self.exec + ";\n"
	}

	source += "}\n";

	return source;
}

func compile(w *World) *bytes.Buffer {
	ioutil.WriteFile(TEMPPATH+".go", strings.Bytes(w.source()), 0644);

	err := new(bytes.Buffer);

	re, e, _ := os.Pipe();

	os.ForkExec(
		bin+"/"+arch+"g",
		[]string{bin + "/" + arch + "g", "-o", TEMPPATH + ".6", TEMPPATH + ".go"},
		os.Environ(),
		"",
		[]*os.File{nil, e, nil});

	e.Close();
	io.Copy(err, re);

	if err.Len() > 0 {
		return err
	}

	re, e, _ = os.Pipe();
	os.ForkExec(
		bin+"/"+arch+"l",
		[]string{bin + "/" + arch + "l", "-o", TEMPPATH + "", TEMPPATH + ".6"},
		os.Environ(),
		"",
		[]*os.File{nil, e, nil});

	e.Close();
	io.Copy(err, re);

	return err;
}

func run() (*bytes.Buffer, *bytes.Buffer) {
	out := new(bytes.Buffer);
	err := new(bytes.Buffer);

	re, e, _ := os.Pipe();
	ro, o, _ := os.Pipe();
	os.ForkExec(
		TEMPPATH,
		[]string{TEMPPATH},
		os.Environ(),
		"",
		[]*os.File{nil, o, e});

	e.Close();
	io.Copy(err, re);

	if err.Len() > 0 {
		return nil, err
	}

	o.Close();
	io.Copy(out, ro);

	return out, err;
}

func main() {
	fmt.Println("Welcome to the Go REPL!");
	fmt.Println("Enter '?' for a list of commands.");

	w := new(World);
	w.pkgs = new(vector.StringVector);
	w.defs = new(vector.StringVector);
	w.code = new(vector.Vector);

	buf := bufio.NewReader(os.Stdin);
	unstable := false;
	for {
		if unstable {
			fmt.Print("! ");
		}

		fmt.Print(strings.Join(w.pkgs.Data(), " ") + "> ");

		read, err := buf.ReadString('\n');
		if err != nil {
			println();
			break
		}

		line := read[0:len(read) - 1];
		if len(line) == 0 {
			continue
		}

		w.exec = "";

		switch line[0] {
		case '?':
			fmt.Println("Commands:");
			fmt.Println("\t?: help");
			fmt.Println("\t+ (pkgname): import package");
			fmt.Println("\t-[dpc]: pop last (declaration|package|code)");
			fmt.Println("\t~: reset");
			fmt.Println("\t: (expr): add persistent code");
			fmt.Println("\t!: inspect source");
		case '+':
			w.pkgs.Push(line[2:]);
			unstable = true;
		case '-':
			if len(line) > 1 {
				switch line[1] {
				case 'd':
					if w.defs.Len() > 0 {
						w.defs.Pop()
					}
				case 'p':
					if w.pkgs.Len() > 0 {
						if len(line) > 3 {
							for i, v := range w.pkgs.Data() {
								if v == line[3:] {
									w.pkgs.Delete(i);
									break;
								}
							}
						} else {
							w.pkgs.Pop()
						}
					}
				case 'c':
					if w.code.Len() > 0 {
						w.code.Pop()
					}
				}
			} else {
				if w.code.Len() > 0 {
					w.code.Pop()
				}
			}

			unstable = compile(w).Len() > 0;
		case '~':
			w.pkgs.Resize(0, 0);
			w.defs.Resize(0, 0);
			w.code.Resize(0, 0);
			unstable = false;
		case '!':
			fmt.Println(w.source())
		case ':':
			tree, err := parser.ParseStmtList("go-repl", line[2:]);
			if err != nil {
				fmt.Println("Parse error:", err);
				continue;
			}

			w.code.Push(tree[0]);

			unstable = compile(w).Len() > 0;
		default:
			var tree interface{}
			tree, err := parser.ParseStmtList("go-repl", line[0:]);
			if err != nil {
				tree, err = parser.ParseDeclList("go-repl", line[0:]);
				if err != nil {
					fmt.Println("Parse error:", err);
					continue;
				}
			}

			switch tree.(type) {
			case []ast.Stmt:
				for _, v := range tree.([]ast.Stmt) {
					str := new(bytes.Buffer);
					printer.Fprint(str, v);

					switch v.(type) {
					case *ast.AssignStmt:
						w.code.Push(v)
					default:
						w.exec = str.String()
					}
				}
			case []ast.Decl:
				for _, v := range tree.([]ast.Decl) {
					str := new(bytes.Buffer);
					printer.Fprint(str, v);

					w.defs.Push(str.String());
				}
			}

			if err := compile(w); err.Len() > 0 {
				fmt.Println("Compile error:", err);
			} else if out, err := run(); err.Len() > 0 {
				fmt.Println("Runtime error:\n", err)
			} else {
				fmt.Print(out)
			}
		}
	}
}
