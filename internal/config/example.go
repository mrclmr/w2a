package config

import (
	"bytes"
	_ "embed"
	"runtime"
	"text/template"
)

//go:embed example.yaml.tmpl
var exampleYamlTmpl string

func Example() (string, error) {
	parse, err := template.New("").
		Delims("[[", "]]").
		Funcs(template.FuncMap{"isDarwin": func() bool { return runtime.GOOS == "darwin" }}).
		Parse(exampleYamlTmpl)
	if err != nil {
		return "", err
	}

	buf := &bytes.Buffer{}
	err = parse.Execute(buf, nil)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
